//go:build !linux

package main

import (
	"fmt"
	"os"
)

// FanotifyMonitor stub for non-Linux systems.
// The real implementation lives in reader.go (linux-only).
type FanotifyMonitor struct{}

func NewFanotifyMonitor(_ []string) *FanotifyMonitor { return &FanotifyMonitor{} }

func (fm *FanotifyMonitor) Init() error {
	fmt.Fprintln(os.Stderr, "ERROR: fanotify is only supported on Linux")
	os.Exit(1)
	return nil
}

func (fm *FanotifyMonitor) Close() error { return nil }

func (fm *FanotifyMonitor) Start(_ chan<- AuditEvent) error {
	return fmt.Errorf("fanotify not available on this platform")
}
