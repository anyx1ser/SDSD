package main

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildFeatureVectorBasicMetrics(t *testing.T) {
	a := NewAggregator(10*time.Second, 2*time.Second)
	base := time.Unix(1000, 0)
	windowEnd := base.Add(10 * time.Second)

	events := []AuditEvent{
		{Timestamp: base.Add(0 * time.Second), UID: "1000", ProcessName: "cat", FilePath: "/home/u/docs/a.txt", EventType: "open", Success: true},
		{Timestamp: base.Add(1 * time.Second), UID: "1000", ProcessName: "cat", FilePath: "/home/u/docs/a.txt", EventType: "read", Success: true},
		{Timestamp: base.Add(2 * time.Second), UID: "1000", ProcessName: "vim", FilePath: "/home/u/docs/b.log", EventType: "close_write", Success: true},
		{Timestamp: base.Add(3 * time.Second), UID: "1000", ProcessName: "cat", FilePath: "/tmp/c.csv", EventType: "read", Success: true},
	}

	fv := a.buildFeatureVector("1000", events, windowEnd)

	if fv.TotalFileAccesses != 4 {
		t.Fatalf("TotalFileAccesses=%d, want 4", fv.TotalFileAccesses)
	}
	if fv.ReadOperations != 2 {
		t.Fatalf("ReadOperations=%d, want 2", fv.ReadOperations)
	}
	if fv.WriteOperations != 1 {
		t.Fatalf("WriteOperations=%d, want 1", fv.WriteOperations)
	}
	if fv.UniqueFilesAccessed != 3 {
		t.Fatalf("UniqueFilesAccessed=%d, want 3", fv.UniqueFilesAccessed)
	}
	if fv.UniqueDirectoriesAccessed != 2 {
		t.Fatalf("UniqueDirectoriesAccessed=%d, want 2", fv.UniqueDirectoriesAccessed)
	}
	if fv.UniqueProcesses != 2 {
		t.Fatalf("UniqueProcesses=%d, want 2", fv.UniqueProcesses)
	}
	if math.Abs(fv.FileAccessRate-0.4) > 1e-9 {
		t.Fatalf("FileAccessRate=%f, want 0.4", fv.FileAccessRate)
	}
	if math.Abs(fv.RatioUniqueFiles-0.75) > 1e-9 {
		t.Fatalf("RatioUniqueFiles=%f, want 0.75", fv.RatioUniqueFiles)
	}
	if math.Abs(fv.AvgTimeBetweenEvents-1.0) > 1e-9 {
		t.Fatalf("AvgTimeBetweenEvents=%f, want 1.0", fv.AvgTimeBetweenEvents)
	}
	if fv.MaxEventsInShortInterval != 2 {
		t.Fatalf("MaxEventsInShortInterval=%d, want 2", fv.MaxEventsInShortInterval)
	}
	if fv.ProcessEntropy <= 0 {
		t.Fatalf("ProcessEntropy=%f, want > 0", fv.ProcessEntropy)
	}
	if fv.FileExtensionEntropy <= 0 {
		t.Fatalf("FileExtensionEntropy=%f, want > 0", fv.FileExtensionEntropy)
	}
	if fv.DirectoryAccessEntropy <= 0 {
		t.Fatalf("DirectoryAccessEntropy=%f, want > 0", fv.DirectoryAccessEntropy)
	}
}

func TestBuildFeatureVectorArchiveSignals(t *testing.T) {
	a := NewAggregator(10*time.Second, 2*time.Second)
	base := time.Unix(2000, 0)
	windowEnd := base.Add(10 * time.Second)

	events := make([]AuditEvent, 0, 32)
	for i := 0; i < 30; i++ {
		events = append(events, AuditEvent{
			Timestamp:   base.Add(time.Duration(i) * 250 * time.Millisecond),
			UID:         "1001",
			ProcessName: "python",
			FilePath:    filepath.Join("/home/u/project", "f", "doc", string(rune('a'+(i%26)))+".txt"),
			EventType:   "read",
			Success:     true,
		})
	}
	// Archive creation after many reads.
	events = append(events, AuditEvent{
		Timestamp:   base.Add(8 * time.Second),
		UID:         "1001",
		ProcessName: "tar",
		FilePath:    "/home/u/project/archive.tar.gz",
		EventType:   "close_write",
		Success:     true,
	})

	fv := a.buildFeatureVector("1001", events, windowEnd)

	if !fv.ArchiveProcessDetected {
		t.Fatal("ArchiveProcessDetected=false, want true")
	}
	if !fv.ArchiveOutputCreated {
		t.Fatal("ArchiveOutputCreated=false, want true")
	}
	if fv.FilesReadBeforeArchive < 20 {
		t.Fatalf("FilesReadBeforeArchive=%d, want >= 20", fv.FilesReadBeforeArchive)
	}
	if !fv.ArchiveLikeAccessPattern {
		t.Fatal("ArchiveLikeAccessPattern=false, want true")
	}
}

func TestBuildFeatureVectorTransferProcessSignal(t *testing.T) {
	a := NewAggregator(10*time.Second, 2*time.Second)
	base := time.Unix(3000, 0)
	windowEnd := base.Add(10 * time.Second)

	events := []AuditEvent{
		{Timestamp: base.Add(0 * time.Second), UID: "1002", ProcessName: "rsync", FilePath: "/home/u/docs/a.txt", EventType: "read", Success: true},
		{Timestamp: base.Add(200 * time.Millisecond), UID: "1002", ProcessName: "rsync", FilePath: "/home/u/docs/b.txt", EventType: "read", Success: true},
	}

	fv := a.buildFeatureVector("1002", events, windowEnd)
	if !fv.TransferProcessDetected {
		t.Fatal("TransferProcessDetected=false, want true")
	}
}

func TestToFeatureArrayBooleanEncoding(t *testing.T) {
	fv := FeatureVector{
		ArchiveProcessDetected:   true,
		ArchiveOutputCreated:     false,
		ArchiveLikeAccessPattern: true,
	}
	arr := fv.ToFeatureArray()

	if len(arr) != 18 {
		t.Fatalf("feature array len=%d, want 18", len(arr))
	}
	if arr[12] != 1.0 {
		t.Fatalf("archive_process_encoded=%f, want 1.0", arr[12])
	}
	if arr[13] != 0.0 {
		t.Fatalf("archive_output_encoded=%f, want 0.0", arr[13])
	}
	if arr[14] != 1.0 {
		t.Fatalf("archive_pattern_encoded=%f, want 1.0", arr[14])
	}
}
