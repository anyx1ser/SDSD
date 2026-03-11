package main

import "time"

// AuditEvent represents a parsed audit log event.
type AuditEvent struct {
	Timestamp   time.Time
	UID         string
	ProcessName string
	FilePath    string
	EventType   string // "open" | "read" | "execve" | "close_write"
	Success     bool
}

// FeatureVector represents the aggregated feature set for one time-window.
// These four counters are fed directly into the Isolation Forest model.
type FeatureVector struct {
	Timestamp       time.Time
	WindowIndex     int
	UID             string  // user this window belongs to
	FileAccessCount int     // open + read operations
	UniqueFileCount int     // distinct files touched
	ReadCount       int     // read() syscall count
	ExecCount       int     // execve() syscall count
	AnomalyScore    float64 // IsolationForest anomaly score (0–1, higher = more anomalous)
	IsAnomaly       bool    // true when score exceeds the per-user threshold
}

// ToFeatureArray returns the four numeric features as a plain slice for the ML model.
func (fv FeatureVector) ToFeatureArray() []float64 {
	return []float64{
		float64(fv.FileAccessCount),
		float64(fv.UniqueFileCount),
		float64(fv.ReadCount),
		float64(fv.ExecCount),
	}
}

// AnomalyAlert represents a detected anomaly emitted by the detector.
type AnomalyAlert struct {
	Time        time.Time
	WindowIndex int
	UID         string
	Score       float64
	Reason      string
	Features    FeatureVector
}

// UserBaseline holds the persisted, trained Isolation Forest model for one UID.
// ModelJSON contains the JSON-serialised IsolationForest struct.
type UserBaseline struct {
	UID           string
	TrainedAt     time.Time
	NSamples      int     // number of windows used for training
	Contamination float64 // IsolationForest contamination parameter
	ModelJSON     []byte  // JSON blob of the serialised model
}
