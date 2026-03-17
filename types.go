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
// It contains behavioral, diversity, temporal, archive, and entropy features.
type FeatureVector struct {
	Timestamp   time.Time
	WindowIndex int
	UID         string

	// Basic activity features.
	TotalFileAccesses int
	ReadOperations    int
	WriteOperations   int
	FileAccessRate    float64

	// Filesystem diversity features.
	UniqueFilesAccessed       int
	UniqueDirectoriesAccessed int
	RatioUniqueFiles          float64

	// Process behavior features.
	UniqueProcesses int
	ProcessEntropy  float64

	// Temporal features.
	AvgTimeBetweenEvents     float64
	MaxEventsInShortInterval int

	// Archiving detection features.
	FilesReadBeforeArchive   int
	ArchiveProcessDetected   bool
	ArchiveOutputCreated     bool
	ArchiveLikeAccessPattern bool
	TransferProcessDetected  bool

	// Entropy features.
	FileExtensionEntropy   float64
	DirectoryAccessEntropy float64
	FilenameEntropy        float64

	AnomalyScore float64
	IsAnomaly    bool
}

// ToFeatureArray returns the numeric features used by anomaly models.
func (fv FeatureVector) ToFeatureArray() []float64 {
	archiveProc := 0.0
	if fv.ArchiveProcessDetected {
		archiveProc = 1.0
	}
	archiveOut := 0.0
	if fv.ArchiveOutputCreated {
		archiveOut = 1.0
	}
	archivePattern := 0.0
	if fv.ArchiveLikeAccessPattern {
		archivePattern = 1.0
	}

	return []float64{
		float64(fv.TotalFileAccesses),
		float64(fv.ReadOperations),
		float64(fv.WriteOperations),
		fv.FileAccessRate,
		float64(fv.UniqueFilesAccessed),
		float64(fv.UniqueDirectoriesAccessed),
		fv.RatioUniqueFiles,
		float64(fv.UniqueProcesses),
		fv.ProcessEntropy,
		fv.AvgTimeBetweenEvents,
		float64(fv.MaxEventsInShortInterval),
		float64(fv.FilesReadBeforeArchive),
		archiveProc,
		archiveOut,
		archivePattern,
		fv.FileExtensionEntropy,
		fv.DirectoryAccessEntropy,
		fv.FilenameEntropy,
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
	NSamples      int
	Contamination float64
	ZThreshold    float64
	MeanJSON      []byte
	VarianceJSON  []byte
	ModelJSON     []byte
}
