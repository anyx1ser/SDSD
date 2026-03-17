package main

// database.go
//
// SQLite persistence layer for SDSD.
//
// Three tables
// ------------
//  baselines       – one row per UID; stores the serialised Isolation Forest
//                   model (JSON) and training metadata.
//  feature_vectors – rolling history of every processed window (useful for
//                   offline analysis, re-training, and audit trails).
//  alerts          – every anomaly alert with full feature context.
//
// This file is the only place in the codebase that imports database/sql and
// modernc.org/sqlite.  All other packages interact via the Database type.
//
// Driver: modernc.org/sqlite (pure Go, no CGO required).

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// ──────────────────────────────────────────────────────────────────────────────
// DDL
// ──────────────────────────────────────────────────────────────────────────────

const dbDDL = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS baselines (
    uid             TEXT    PRIMARY KEY,
    trained_at      TEXT    NOT NULL,   -- RFC-3339 timestamp
    n_samples       INTEGER NOT NULL,
    contamination   REAL    NOT NULL,
		threshold       REAL    NOT NULL,   -- reserved for compatibility
		z_threshold     REAL    NOT NULL DEFAULT 0,
		mean_json       BLOB,
		variance_json   BLOB,
    model_json      BLOB    NOT NULL    -- JSON-serialised IsolationForest
);

CREATE TABLE IF NOT EXISTS feature_vectors (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    recorded_at         TEXT    NOT NULL,
    window_index        INTEGER NOT NULL,
    uid                 TEXT    NOT NULL,
		file_access_count   INTEGER NOT NULL DEFAULT 0,
		unique_file_count   INTEGER NOT NULL DEFAULT 0,
		read_count          INTEGER NOT NULL DEFAULT 0,
		write_count         INTEGER NOT NULL DEFAULT 0,
		unique_dir_count    INTEGER NOT NULL DEFAULT 0,
		process_entropy     REAL    NOT NULL DEFAULT 0,
    anomaly_score       REAL    NOT NULL DEFAULT 0.0,
    is_anomaly          INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_fv_uid         ON feature_vectors (uid);
CREATE INDEX IF NOT EXISTS idx_fv_recorded_at ON feature_vectors (recorded_at);

CREATE TABLE IF NOT EXISTS alerts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    alerted_at          TEXT    NOT NULL,
    window_index        INTEGER NOT NULL,
    uid                 TEXT    NOT NULL,
    score               REAL    NOT NULL,
    reason              TEXT    NOT NULL,
    file_access_count   INTEGER NOT NULL DEFAULT 0,
    unique_file_count   INTEGER NOT NULL DEFAULT 0,
    read_count          INTEGER NOT NULL DEFAULT 0,
		write_count         INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_alerts_uid       ON alerts (uid);
CREATE INDEX IF NOT EXISTS idx_alerts_alerted_at ON alerts (alerted_at);
`

// ──────────────────────────────────────────────────────────────────────────────
// Database
// ──────────────────────────────────────────────────────────────────────────────

// Database wraps a single SQLite connection.  It is safe for concurrent use
// because all writes are serialised via mu.
type Database struct {
	db *sql.DB
	mu sync.Mutex
}

// NewDatabase opens (or creates) the SQLite file at path and initialises the
// schema.  Pass ":memory:" for an in-memory database useful in tests.
func NewDatabase(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	// SQLite is not thread-safe with multiple concurrent writers; one connection
	// serialised by our mutex is the simplest safe approach.
	db.SetMaxOpenConns(1)

	d := &Database{db: db}
	if err := d.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

func (d *Database) initSchema() error {
	if _, err := d.db.Exec(dbDDL); err != nil {
		return err
	}
	if err := d.ensureColumn("baselines", "z_threshold", "ALTER TABLE baselines ADD COLUMN z_threshold REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("baselines", "mean_json", "ALTER TABLE baselines ADD COLUMN mean_json BLOB"); err != nil {
		return err
	}
	if err := d.ensureColumn("baselines", "variance_json", "ALTER TABLE baselines ADD COLUMN variance_json BLOB"); err != nil {
		return err
	}
	if err := d.ensureColumn("feature_vectors", "write_count", "ALTER TABLE feature_vectors ADD COLUMN write_count INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("feature_vectors", "unique_dir_count", "ALTER TABLE feature_vectors ADD COLUMN unique_dir_count INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("feature_vectors", "process_entropy", "ALTER TABLE feature_vectors ADD COLUMN process_entropy REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := d.ensureColumn("alerts", "write_count", "ALTER TABLE alerts ADD COLUMN write_count INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

func (d *Database) ensureColumn(table, column, alterSQL string) error {
	rows, err := d.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = d.db.Exec(alterSQL)
	return err
}

// Close shuts down the database connection.
func (d *Database) Close() error {
	return d.db.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// Baselines CRUD
// ──────────────────────────────────────────────────────────────────────────────

// SaveBaseline inserts or replaces the trained model for a UID.
func (d *Database) SaveBaseline(b UserBaseline) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	const q = `
		INSERT INTO baselines (uid, trained_at, n_samples, contamination, threshold, z_threshold, mean_json, variance_json, model_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			trained_at    = excluded.trained_at,
			n_samples     = excluded.n_samples,
			contamination = excluded.contamination,
			threshold     = excluded.threshold,
			z_threshold   = excluded.z_threshold,
			mean_json     = excluded.mean_json,
			variance_json = excluded.variance_json,
			model_json    = excluded.model_json`

	_, err := d.db.Exec(q,
		b.UID,
		b.TrainedAt.UTC().Format(time.RFC3339),
		b.NSamples,
		b.Contamination,
		0.0,
		b.ZThreshold,
		b.MeanJSON,
		b.VarianceJSON,
		b.ModelJSON,
	)
	return err
}

// LoadBaseline returns the persisted baseline for uid, or (zero, nil) when absent.
func (d *Database) LoadBaseline(uid string) (UserBaseline, bool, error) {
	const q = `SELECT uid, trained_at, n_samples, contamination, z_threshold, mean_json, variance_json, model_json
	           FROM baselines WHERE uid = ?`

	row := d.db.QueryRow(q, uid)
	var b UserBaseline
	var trainedAtStr string
	err := row.Scan(&b.UID, &trainedAtStr, &b.NSamples, &b.Contamination, &b.ZThreshold, &b.MeanJSON, &b.VarianceJSON, &b.ModelJSON)
	if err == sql.ErrNoRows {
		return UserBaseline{}, false, nil
	}
	if err != nil {
		return UserBaseline{}, false, fmt.Errorf("load baseline uid=%s: %w", uid, err)
	}
	b.TrainedAt, err = time.Parse(time.RFC3339, trainedAtStr)
	if err != nil {
		return UserBaseline{}, false, fmt.Errorf("parse trained_at: %w", err)
	}
	return b, true, nil
}

// ListBaselineUIDs returns every UID that has a stored baseline.
func (d *Database) ListBaselineUIDs() ([]string, error) {
	rows, err := d.db.Query("SELECT uid FROM baselines ORDER BY uid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		uids = append(uids, uid)
	}
	return uids, rows.Err()
}

// DeleteBaseline removes the stored model for uid (force re-training).
func (d *Database) DeleteBaseline(uid string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec("DELETE FROM baselines WHERE uid = ?", uid)
	return err
}

// ──────────────────────────────────────────────────────────────────────────────
// Feature-vector history
// ──────────────────────────────────────────────────────────────────────────────

// InsertFeatureVector appends a processed window to the rolling history.
func (d *Database) InsertFeatureVector(fv FeatureVector) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	const q = `
		INSERT INTO feature_vectors
			(recorded_at, window_index, uid,
			 file_access_count, unique_file_count, read_count, write_count, unique_dir_count, process_entropy,
			 anomaly_score, is_anomaly)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.db.Exec(q,
		fv.Timestamp.UTC().Format(time.RFC3339),
		fv.WindowIndex,
		fv.UID,
		fv.TotalFileAccesses,
		fv.UniqueFilesAccessed,
		fv.ReadOperations,
		fv.WriteOperations,
		fv.UniqueDirectoriesAccessed,
		fv.ProcessEntropy,
		fv.AnomalyScore,
		boolToInt(fv.IsAnomaly),
	)
	return err
}

// GetFeatureVectorsForUID returns the most-recent limit rows for a UID,
// suitable for offline re-training.
func (d *Database) GetFeatureVectorsForUID(uid string, limit int) ([]FeatureVector, error) {
	const q = `
		SELECT recorded_at, window_index, uid,
		       file_access_count, unique_file_count, read_count, write_count, unique_dir_count, process_entropy,
		       anomaly_score, is_anomaly
		FROM feature_vectors
		WHERE uid = ?
		ORDER BY recorded_at DESC
		LIMIT ?`

	rows, err := d.db.Query(q, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FeatureVector
	for rows.Next() {
		var fv FeatureVector
		var recAt string
		var isAnomaly int
		if err := rows.Scan(
			&recAt, &fv.WindowIndex, &fv.UID,
			&fv.TotalFileAccesses, &fv.UniqueFilesAccessed, &fv.ReadOperations, &fv.WriteOperations, &fv.UniqueDirectoriesAccessed, &fv.ProcessEntropy,
			&fv.AnomalyScore, &isAnomaly,
		); err != nil {
			return nil, err
		}
		fv.Timestamp, _ = time.Parse(time.RFC3339, recAt)
		fv.IsAnomaly = isAnomaly != 0
		result = append(result, fv)
	}
	return result, rows.Err()
}

// PruneFeatureVectors deletes old rows to keep the DB size bounded.
func (d *Database) PruneFeatureVectors(keepLastN int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`
		DELETE FROM feature_vectors
		WHERE id NOT IN (
			SELECT id FROM feature_vectors ORDER BY id DESC LIMIT ?
		)`, keepLastN)
	return err
}

// ──────────────────────────────────────────────────────────────────────────────
// Alerts
// ──────────────────────────────────────────────────────────────────────────────

// InsertAlert persists a detected anomaly alert.
func (d *Database) InsertAlert(a AnomalyAlert) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	const q = `
		INSERT INTO alerts
			(alerted_at, window_index, uid, score, reason,
			 file_access_count, unique_file_count, read_count, write_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	fv := a.Features
	_, err := d.db.Exec(q,
		a.Time.UTC().Format(time.RFC3339),
		a.WindowIndex,
		a.UID,
		a.Score,
		a.Reason,
		fv.TotalFileAccesses,
		fv.UniqueFilesAccessed,
		fv.ReadOperations,
		fv.WriteOperations,
	)
	return err
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
