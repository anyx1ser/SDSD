package main

import (
	"path/filepath"
	"strings"
	"time"
)

type uidEventBuffer struct {
	events []AuditEvent
}

// Aggregator collects events and emits per-UID feature vectors over
// overlapping windows (windowSize with step interval).
type Aggregator struct {
	windowSize   time.Duration
	step         time.Duration
	shortBurst   time.Duration
	uidBuffers   map[string]*uidEventBuffer
	eventsChan   chan AuditEvent
	featuresChan chan FeatureVector
	windowIndex  int
}

// NewAggregator creates a real-time sliding-window aggregator.
func NewAggregator(windowSize, step time.Duration) *Aggregator {
	if windowSize <= 0 {
		windowSize = 10 * time.Second
	}
	if step <= 0 {
		step = 2 * time.Second
	}

	return &Aggregator{
		windowSize:   windowSize,
		step:         step,
		shortBurst:   time.Second,
		uidBuffers:   make(map[string]*uidEventBuffer),
		eventsChan:   make(chan AuditEvent, 8192),
		featuresChan: make(chan FeatureVector, 2048),
	}
}

func (a *Aggregator) GetEventsChan() chan<- AuditEvent {
	return a.eventsChan
}

func (a *Aggregator) GetFeaturesChan() <-chan FeatureVector {
	return a.featuresChan
}

// Start runs the ingestion + periodic sliding-window flush loop.
func (a *Aggregator) Start() {
	ticker := time.NewTicker(a.step)
	defer ticker.Stop()

	for {
		select {
		case event := <-a.eventsChan:
			if !event.Success {
				continue
			}
			buf, ok := a.uidBuffers[event.UID]
			if !ok {
				buf = &uidEventBuffer{}
				a.uidBuffers[event.UID] = buf
			}
			buf.events = append(buf.events, event)
		case tick := <-ticker.C:
			a.flushWindow(tick)
		}
	}
}

func (a *Aggregator) flushWindow(windowEnd time.Time) {
	windowStart := windowEnd.Add(-a.windowSize)
	for uid, buf := range a.uidBuffers {
		if len(buf.events) == 0 {
			continue
		}

		cut := 0
		for cut < len(buf.events) && buf.events[cut].Timestamp.Before(windowStart) {
			cut++
		}
		if cut > 0 {
			buf.events = buf.events[cut:]
		}
		if len(buf.events) == 0 {
			continue
		}

		startIdx := 0
		for startIdx < len(buf.events) && buf.events[startIdx].Timestamp.Before(windowStart) {
			startIdx++
		}
		endIdx := len(buf.events)
		for endIdx > startIdx && buf.events[endIdx-1].Timestamp.After(windowEnd) {
			endIdx--
		}
		if endIdx <= startIdx {
			continue
		}

		fv := a.buildFeatureVector(uid, buf.events[startIdx:endIdx], windowEnd)
		if fv.TotalFileAccesses == 0 && fv.ReadOperations == 0 && fv.WriteOperations == 0 {
			continue
		}

		select {
		case a.featuresChan <- fv:
		default:
			// Drop if detector is overloaded.
		}
		a.windowIndex++
	}
}

func (a *Aggregator) buildFeatureVector(uid string, events []AuditEvent, windowEnd time.Time) FeatureVector {
	uniqueFiles := make(map[string]struct{}, len(events))
	uniqueDirs := make(map[string]struct{}, len(events))
	processCounts := make(map[string]int, 8)
	extensionCounts := make(map[string]int, 8)
	dirCounts := make(map[string]int, 8)
	filenamePatternCounts := make(map[string]int, 8)
	filesReadByDir := make(map[string]map[string]struct{}, 8)

	var (
		totalAccesses          int
		readOps                int
		writeOps               int
		totalGap               float64
		gapCount               int
		maxBurst               int
		archiveProcess         bool
		transferProcess        bool
		archiveOutputCreated   bool
		filesReadBeforeArchive int
	)

	firstArchiveIdx := -1
	for i := range events {
		e := events[i]
		op := normalizeOperation(e.EventType)

		if isArchiveProcess(e.ProcessName) {
			archiveProcess = true
			if firstArchiveIdx == -1 {
				firstArchiveIdx = i
			}
		}
		if isTransferProcess(e.ProcessName) {
			transferProcess = true
		}

		if op == "open" || op == "read" || op == "write" {
			totalAccesses++
		}
		if op == "read" {
			readOps++
		}
		if op == "write" {
			writeOps++
		}

		if i > 0 {
			delta := e.Timestamp.Sub(events[i-1].Timestamp).Seconds()
			if delta >= 0 {
				totalGap += delta
				gapCount++
			}
		}

		if e.FilePath != "" {
			uniqueFiles[e.FilePath] = struct{}{}
			dir := filepath.Dir(e.FilePath)
			base := filepath.Base(e.FilePath)
			ext := strings.ToLower(filepath.Ext(base))
			if ext == "" {
				ext = "(none)"
			}

			uniqueDirs[dir] = struct{}{}
			dirCounts[dir]++
			extensionCounts[ext]++
			filenamePatternCounts[FilenamePattern(base)]++

			if op == "read" {
				set, ok := filesReadByDir[dir]
				if !ok {
					set = make(map[string]struct{}, 16)
					filesReadByDir[dir] = set
				}
				set[e.FilePath] = struct{}{}
			}

			if op == "write" && isArchiveOutputPath(e.FilePath) {
				archiveOutputCreated = true
			}
		}

		if e.ProcessName != "" {
			processCounts[e.ProcessName]++
		}
	}

	if firstArchiveIdx > 0 {
		for i := 0; i < firstArchiveIdx; i++ {
			if normalizeOperation(events[i].EventType) == "read" {
				filesReadBeforeArchive++
			}
		}
	}

	for i := 0; i < len(events); i++ {
		j := i
		for j < len(events) && events[j].Timestamp.Sub(events[i].Timestamp) <= a.shortBurst {
			j++
		}
		if j-i > maxBurst {
			maxBurst = j - i
		}
	}

	maxReadFilesInSingleDir := 0
	for _, files := range filesReadByDir {
		if len(files) > maxReadFilesInSingleDir {
			maxReadFilesInSingleDir = len(files)
		}
	}

	ratioUniqueFiles := 0.0
	if totalAccesses > 0 {
		ratioUniqueFiles = float64(len(uniqueFiles)) / float64(totalAccesses)
	}

	avgGap := 0.0
	if gapCount > 0 {
		avgGap = totalGap / float64(gapCount)
	}

	archiveLikePattern := maxReadFilesInSingleDir >= 20 && readOps >= 30 && ratioUniqueFiles >= 0.6
	if filesReadBeforeArchive >= 20 && archiveOutputCreated {
		archiveLikePattern = true
	}

	windowSeconds := a.windowSize.Seconds()
	if windowSeconds <= 0 {
		windowSeconds = 10
	}

	return FeatureVector{
		Timestamp:                 windowEnd,
		WindowIndex:               a.windowIndex,
		UID:                       uid,
		TotalFileAccesses:         totalAccesses,
		ReadOperations:            readOps,
		WriteOperations:           writeOps,
		FileAccessRate:            float64(totalAccesses) / windowSeconds,
		UniqueFilesAccessed:       len(uniqueFiles),
		UniqueDirectoriesAccessed: len(uniqueDirs),
		RatioUniqueFiles:          ratioUniqueFiles,
		UniqueProcesses:           len(processCounts),
		ProcessEntropy:            ShannonEntropy(processCounts),
		AvgTimeBetweenEvents:      avgGap,
		MaxEventsInShortInterval:  maxBurst,
		FilesReadBeforeArchive:    filesReadBeforeArchive,
		ArchiveProcessDetected:    archiveProcess,
		TransferProcessDetected:   transferProcess,
		ArchiveOutputCreated:      archiveOutputCreated,
		ArchiveLikeAccessPattern:  archiveLikePattern,
		FileExtensionEntropy:      ShannonEntropy(extensionCounts),
		DirectoryAccessEntropy:    ShannonEntropy(dirCounts),
		FilenameEntropy:           ShannonEntropy(filenamePatternCounts),
	}
}

func normalizeOperation(eventType string) string {
	switch eventType {
	case "close_write", "write":
		return "write"
	case "open":
		return "open"
	case "read", "access":
		return "read"
	default:
		return eventType
	}
}

func isArchiveProcess(proc string) bool {
	switch strings.ToLower(strings.TrimSpace(proc)) {
	case "tar", "zip", "gzip", "7z", "7za", "7zr", "bsdtar":
		return true
	default:
		return false
	}
}

func isArchiveOutputPath(path string) bool {
	l := strings.ToLower(path)
	return strings.HasSuffix(l, ".tar") || strings.HasSuffix(l, ".tar.gz") ||
		strings.HasSuffix(l, ".tgz") || strings.HasSuffix(l, ".zip") ||
		strings.HasSuffix(l, ".7z") || strings.HasSuffix(l, ".gz") ||
		strings.HasSuffix(l, ".bz2") || strings.HasSuffix(l, ".xz")
}

func isTransferProcess(proc string) bool {
	switch strings.ToLower(strings.TrimSpace(proc)) {
	case "rsync", "rclone", "scp", "sftp":
		return true
	default:
		return false
	}
}
