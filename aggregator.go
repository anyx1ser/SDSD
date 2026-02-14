package main

import (
	"time"
)

// Aggregator collects audit events and aggregates them into feature vectors
type Aggregator struct {
	windowSize      time.Duration
	currentWindow   *windowData
	eventsChan      chan AuditEvent
	featuresChan    chan FeatureVector
	windowIndex     int
}

// windowData tracks data for current aggregation window
type windowData struct {
	startTime     time.Time
	fileAccesses  map[string]bool // Track unique file accesses
	readOps       int
	openOps       int
	execOps       int
	totalOps      int
}

// NewAggregator creates a new aggregator with specified window size
// Typically 10 seconds per window as per requirements
func NewAggregator(windowSize time.Duration) *Aggregator {
	return &Aggregator{
		windowSize:   windowSize,
		eventsChan:   make(chan AuditEvent, 1000),
		featuresChan: make(chan FeatureVector, 100),
		windowIndex:  0,
	}
}

// GetEventsChan returns the channel for sending audit events to aggregator
func (a *Aggregator) GetEventsChan() chan<- AuditEvent {
	return a.eventsChan
}

// GetFeaturesChan returns the channel for receiving aggregated feature vectors
func (a *Aggregator) GetFeaturesChan() <-chan FeatureVector {
	return a.featuresChan
}

// Start begins the aggregation process
// Should be run in a goroutine
func (a *Aggregator) Start() {
	a.currentWindow = a.newWindow()

	windowTicker := time.NewTicker(a.windowSize)
	defer windowTicker.Stop()

	for {
		select {
		case event := <-a.eventsChan:
			// Check if we need to flush current window
			if event.Timestamp.Sub(a.currentWindow.startTime) >= a.windowSize {
				// Emit current window
				a.emitWindow()
				// Start new window
				a.currentWindow = a.newWindow()
			}

			// Add event to current window
			a.addEventToWindow(event)

		case <-windowTicker.C:
			// Periodic window flush (in case no events for a while)
			if a.currentWindow.totalOps > 0 {
				a.emitWindow()
				a.currentWindow = a.newWindow()
			}
		}
	}
}

// newWindow creates a fresh window data structure
func (a *Aggregator) newWindow() *windowData {
	return &windowData{
		startTime:    time.Now(),
		fileAccesses: make(map[string]bool),
		readOps:      0,
		openOps:      0,
		execOps:      0,
		totalOps:     0,
	}
}

// addEventToWindow adds an event to the current aggregation window
func (a *Aggregator) addEventToWindow(event AuditEvent) {
	if !event.Success {
		return // Ignore failed operations
	}

	a.currentWindow.totalOps++

	switch event.EventType {
	case "open":
		a.currentWindow.openOps++
		if event.FilePath != "" {
			a.currentWindow.fileAccesses[event.FilePath] = true
		}
	case "read":
		a.currentWindow.readOps++
		if event.FilePath != "" {
			a.currentWindow.fileAccesses[event.FilePath] = true
		}
	case "execve":
		a.currentWindow.execOps++
	}
}

// emitWindow creates a feature vector from the current window and sends it
func (a *Aggregator) emitWindow() {
	features := FeatureVector{
		Timestamp:       a.currentWindow.startTime,
		WindowIndex:     a.windowIndex,
		FileAccessCount: a.currentWindow.openOps + a.currentWindow.readOps,
		UniqueFileCount: len(a.currentWindow.fileAccesses),
		ReadCount:       a.currentWindow.readOps,
		ExecCount:       a.currentWindow.execOps,
		AnomalyScore:    0.0,
		IsAnomaly:       false,
	}

	select {
	case a.featuresChan <- features:
	default:
		// Channel full, drop oldest (shouldn't happen in normal operation)
	}

	a.windowIndex++
}
