package main

// detector.go
//
// ML-based anomaly detector using Isolation Forest (per-user baselines).
//
// Lifecycle for each UID
// ----------------------
//  Phase 1 – Baseline collection
//    The first <baselineWindowCount> FeatureVectors for a UID are accumulated
//    as raw training samples.  They are also persisted to the feature_vectors
//    table for audit/re-training purposes.
//
//  Phase 2 – Training
//    When enough samples have been collected the IsolationForest is trained,
//    the decision threshold is derived from the training data at the specified
//    contamination level, and the serialised model is saved to the baselines
//    table.
//
//  Phase 3 – Detection
//    Every subsequent FeatureVector is scored.  If the anomaly score ≥ the
//    per-user threshold an AnomalyAlert is emitted and written to the alerts
//    table.  The feature vector is also persisted regardless of its label.
//
// On startup the detector tries to load an existing baseline from the DB for
// each UID it encounters; if one is found the UID skips straight to phase 3.

import (
	"fmt"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Per-UID state
// ──────────────────────────────────────────────────────────────────────────────

type detectorPhase int

const (
	phaseCollecting detectorPhase = iota // accumulating baseline samples
	phaseDetecting                       // model trained, scoring live windows
)

// perUIDState holds everything the detector needs for one user.
type perUIDState struct {
	phase         detectorPhase
	trainingData  [][]float64      // raw feature arrays collected during phase 1
	model         *IsolationForest // nil until training is complete
	windowsScored int
}

// ──────────────────────────────────────────────────────────────────────────────
// AnomalyDetector
// ──────────────────────────────────────────────────────────────────────────────

// AnomalyDetector reads FeatureVectors, manages a per-UID IsolationForest, and
// emits AnomalyAlerts.
type AnomalyDetector struct {
	// Hyper-parameters
	baselineWindowCount int
	numTrees            int
	sampleSize          int
	contamination       float64

	// Channels
	featuresChan <-chan FeatureVector
	alertsChan   chan<- AnomalyAlert

	// SQLite persistence (nil disables persistence, useful in tests)
	db *Database

	// Per-UID runtime state (uid -> state)
	uids map[string]*perUIDState
}

// NewAnomalyDetector creates a detector with the given hyper-parameters.
func NewAnomalyDetector(
	featuresChan <-chan FeatureVector,
	alertsChan chan<- AnomalyAlert,
	db *Database,
	baselineWindowCount int,
	numTrees int,
	sampleSize int,
	contamination float64,
) *AnomalyDetector {
	return &AnomalyDetector{
		baselineWindowCount: baselineWindowCount,
		numTrees:            numTrees,
		sampleSize:          sampleSize,
		contamination:       contamination,
		featuresChan:        featuresChan,
		alertsChan:          alertsChan,
		db:                  db,
		uids:                make(map[string]*perUIDState),
	}
}

// Start runs the detection loop.  Call in a goroutine.
func (ad *AnomalyDetector) Start() {
	for fv := range ad.featuresChan {
		ad.processWindow(fv)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Core logic
// ──────────────────────────────────────────────────────────────────────────────

// processWindow handles one incoming FeatureVector.
func (ad *AnomalyDetector) processWindow(fv FeatureVector) {
	uid := fv.UID

	// First time we see this UID: try to load a persisted baseline.
	if _, exists := ad.uids[uid]; !exists {
		ad.uids[uid] = ad.loadOrInitUID(uid)
	}

	state := ad.uids[uid]

	switch state.phase {
	case phaseCollecting:
		ad.collectBaseline(state, &fv)
	case phaseDetecting:
		ad.scoreWindow(state, &fv)
		state.windowsScored++
	}

	// Always persist the feature vector for audit trail / future re-training.
	if ad.db != nil {
		if err := ad.db.InsertFeatureVector(fv); err != nil {
			fmt.Printf("[WARN] DB insert feature vector: %v\n", err)
		}
	}
}

// loadOrInitUID tries to load a trained model from the DB.
// If none exists it returns a fresh collecting-phase state.
func (ad *AnomalyDetector) loadOrInitUID(uid string) *perUIDState {
	if ad.db != nil {
		baseline, found, err := ad.db.LoadBaseline(uid)
		if err != nil {
			fmt.Printf("[WARN] DB load baseline uid=%s: %v\n", uid, err)
		}
		if found {
			model, err := UnmarshalIsolationForest(baseline.ModelJSON)
			if err == nil {
				fmt.Printf("[INFO] Loaded existing baseline for uid=%-6s  trained=%s  n_samples=%d\n",
					uid, baseline.TrainedAt.Format("2006-01-02 15:04:05"), baseline.NSamples)
				return &perUIDState{phase: phaseDetecting, model: model}
			}
			fmt.Printf("[WARN] Corrupted model for uid=%s, re-training: %v\n", uid, err)
		}
	}
	return &perUIDState{phase: phaseCollecting}
}

// collectBaseline accumulates one training sample.
// When enough samples have been gathered it triggers training.
func (ad *AnomalyDetector) collectBaseline(state *perUIDState, fv *FeatureVector) {
	state.trainingData = append(state.trainingData, fv.ToFeatureArray())
	n := len(state.trainingData)

	fmt.Printf("[INFO] Baseline uid=%-6s  collecting %d/%d\n",
		fv.UID, n, ad.baselineWindowCount)

	if n >= ad.baselineWindowCount {
		ad.trainAndSave(state, fv.UID)
	}
}

// trainAndSave trains the IsolationForest on the accumulated data, persists
// the model to SQLite, and transitions the UID to detection phase.
func (ad *AnomalyDetector) trainAndSave(state *perUIDState, uid string) {
	forest := NewIsolationForest(ad.numTrees, ad.sampleSize, ad.contamination)
	forest.Fit(state.trainingData)

	state.model = forest
	state.phase = phaseDetecting

	fmt.Printf("[INFO] Training complete  uid=%-6s  trees=%d  sample_size=%d  contamination=%.2f  threshold=%.4f\n",
		uid, ad.numTrees, ad.sampleSize, ad.contamination, forest.Threshold)

	if ad.db != nil {
		blob, err := forest.Marshal()
		if err != nil {
			fmt.Printf("[WARN] Serialise model uid=%s: %v\n", uid, err)
			return
		}
		b := UserBaseline{
			UID:           uid,
			TrainedAt:     time.Now(),
			NSamples:      len(state.trainingData),
			Contamination: ad.contamination,
			ModelJSON:     blob,
		}
		if err := ad.db.SaveBaseline(b); err != nil {
			fmt.Printf("[WARN] DB save baseline uid=%s: %v\n", uid, err)
		} else {
			fmt.Printf("[INFO] Baseline saved to DB  uid=%s\n", uid)
		}
	}

	// Free training memory — no longer needed.
	state.trainingData = nil
}

// scoreWindow uses the trained IsolationForest to evaluate one live window.
func (ad *AnomalyDetector) scoreWindow(state *perUIDState, fv *FeatureVector) {
	isAnomaly, score := state.model.IsAnomaly(fv.ToFeatureArray())
	fv.AnomalyScore = score
	fv.IsAnomaly = isAnomaly

	if isAnomaly {
		ad.emitAlert(*fv, score)
	}
}

// emitAlert builds and dispatches an AnomalyAlert.
func (ad *AnomalyDetector) emitAlert(fv FeatureVector, score float64) {
	reason := buildAlertReason(fv, score)
	alert := AnomalyAlert{
		Time:        time.Now(),
		WindowIndex: fv.WindowIndex,
		UID:         fv.UID,
		Score:       score,
		Reason:      reason,
		Features:    fv,
	}

	select {
	case ad.alertsChan <- alert:
	default:
		// Drop if channel is full
	}

	if ad.db != nil {
		if err := ad.db.InsertAlert(alert); err != nil {
			fmt.Printf("[WARN] DB insert alert: %v\n", err)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// buildAlertReason generates a human-readable description of what stood out.
func buildAlertReason(fv FeatureVector, score float64) string {
	parts := []string{}
	if fv.FileAccessCount > 0 {
		parts = append(parts, fmt.Sprintf("file_accesses=%d", fv.FileAccessCount))
	}
	if fv.UniqueFileCount > 0 {
		parts = append(parts, fmt.Sprintf("unique_files=%d", fv.UniqueFileCount))
	}
	if fv.ReadCount > 0 {
		parts = append(parts, fmt.Sprintf("reads=%d", fv.ReadCount))
	}
	if fv.ExecCount > 0 {
		parts = append(parts, fmt.Sprintf("execs=%d", fv.ExecCount))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("IsolationForest anomaly score=%.4f", score)
	}
	return fmt.Sprintf("IsolationForest score=%.4f [%s]", score, strings.Join(parts, " "))
}
