package main

// aggregator.go
//
// Collects AuditEvents from the reader, groups them into fixed-size time
// windows, and emits one FeatureVector per (UID, window) pair.
//
// Per-UID windowing is required so that the ML detector can build a separate
// Isolation Forest baseline for each system user.

import (
	"time"
)

// windowData tracks raw counters for one UID inside one time-window.
type windowData struct {
	uid          string
	startTime    time.Time
	fileAccesses map[string]bool // set of unique file paths touched
	readOps      int
	openOps      int
	execOps      int
}

func (w *windowData) totalOps() int {
	return w.readOps + w.openOps + w.execOps
}

// Aggregator collects audit events and emits per-UID FeatureVectors.
type Aggregator struct {
	windowSize   time.Duration
	// uid -> current open window for that user
	uidWindows   map[string]*windowData
	eventsChan   chan AuditEvent
	featuresChan chan FeatureVector
	windowIndex  int
}

// NewAggregator creates an aggregator with the given window duration.
func NewAggregator(windowSize time.Duration) *Aggregator {
	return &Aggregator{
		windowSize:   windowSize,
		uidWindows:   make(map[string]*windowData),
		eventsChan:   make(chan AuditEvent, 1000),
		featuresChan: make(chan FeatureVector, 100),
		windowIndex:  0,
	}
}

// GetEventsChan returns the write-end of the events channel.
func (a *Aggregator) GetEventsChan() chan<- AuditEvent {
	return a.eventsChan
}

// GetFeaturesChan returns the read-end of the features channel.
func (a *Aggregator) GetFeaturesChan() <-chan FeatureVector {
	return a.featuresChan
}

// Start runs the aggregation loop.  Should be called in a goroutine.
func (a *Aggregator) Start() {
	windowTicker := time.NewTicker(a.windowSize)
	defer windowTicker.Stop()

	for {
		select {
		case event := <-a.eventsChan:
			if !event.Success {
				continue
			}
			uid := event.UID

			// Open a fresh window for this UID if none exists
			if _, ok := a.uidWindows[uid]; !ok {
				a.uidWindows[uid] = a.newWindow(uid)
			}

			win := a.uidWindows[uid]

			// Flush if the event falls outside the current window
			if event.Timestamp.Sub(win.startTime) >= a.windowSize {
				a.emitWindow(win)
				a.uidWindows[uid] = a.newWindow(uid)
				win = a.uidWindows[uid]
			}

			a.addEventToWindow(win, event)

		case <-windowTicker.C:
			// Periodically flush all non-empty user windows
			for uid, win := range a.uidWindows {
				if win.totalOps() > 0 {
					a.emitWindow(win)
					a.uidWindows[uid] = a.newWindow(uid)
				}
			}
		}
	}
}

// newWindow allocates a fresh windowData for uid at the current time.
func (a *Aggregator) newWindow(uid string) *windowData {
	return &windowData{
		uid:          uid,
		startTime:    time.Now(),
		fileAccesses: make(map[string]bool),
	}
}

// addEventToWindow routes one event into the appropriate counters.
func (a *Aggregator) addEventToWindow(win *windowData, event AuditEvent) {
	switch event.EventType {
	case "open", "close_write":
		win.openOps++
		if event.FilePath != "" {
			win.fileAccesses[event.FilePath] = true
		}
	case "read":
		win.readOps++
		if event.FilePath != "" {
			win.fileAccesses[event.FilePath] = true
		}
	case "execve":
		win.execOps++
	}
}

// emitWindow builds a FeatureVector and sends it on featuresChan.
func (a *Aggregator) emitWindow(win *windowData) {
	features := FeatureVector{
		Timestamp:       win.startTime,
		WindowIndex:     a.windowIndex,
		UID:             win.uid,
		FileAccessCount: win.openOps + win.readOps,
		UniqueFileCount: len(win.fileAccesses),
		ReadCount:       win.readOps,
		ExecCount:       win.execOps,
		AnomalyScore:    0.0,
		IsAnomaly:       false,
	}
}
