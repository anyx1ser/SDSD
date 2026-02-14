package main

import "time"

// AuditEvent represents a parsed audit log event
type AuditEvent struct {
	Timestamp   time.Time
	UID         string
	ProcessName string
	FilePath    string
	EventType   string // open, read, execve
	Success     bool
}

// FeatureVector represents aggregated features for a time window
type FeatureVector struct {
	Timestamp        time.Time
	WindowIndex      int
	FileAccessCount  int     // Total file access operations
	UniqueFileCount  int     // Unique files accessed
	ReadCount        int     // Read operations
	ExecCount        int     // Executed processes
	AnomalyScore     float64 // Computed anomaly score
	IsAnomaly        bool    // Whether this window is flagged as anomaly
}

// AnomalyAlert represents an anomaly detection alert
type AnomalyAlert struct {
	Time        time.Time
	WindowIndex int
	Score       float64
	Reason      string
	Features    FeatureVector
}

// WindowStats holds statistics for baseline learning
type WindowStats struct {
	FileAccessCount  []float64
	UniqueFileCount  []float64
	ReadCount        []float64
	ExecCount        []float64
	FileAccessMean   float64
	FileAccessStdDev float64
	UniqueFileMean   float64
	UniqueFileStdDev float64
	ReadCountMean    float64
	ReadCountStdDev  float64
	ExecCountMean    float64
	ExecCountStdDev  float64
}
