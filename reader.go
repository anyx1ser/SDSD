package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	FAN_CLASS_NOTIF      = 0x0
	FAN_ACCESS           = 0x1
	FAN_MODIFY           = 0x2
	FAN_CLOSE_WRITE      = 0x8
	FAN_CLOSE_NOWRITE    = 0x10
	FAN_OPEN             = 0x20
	FAN_OPEN_EXEC        = 0x1000
	FAN_ACCESS_PERM      = 0x20000
	FAN_ONDIR            = 0x40000000
	FAN_EVENT_ON_CHILD   = 0x08000000
	FAN_UNLIMITED_QUEUE  = 0x0
	FAN_UNLIMITED_MARKS  = 0x20
	FAN_MARK_ADD         = 0x1
	FAN_MARK_FILESYSTEM  = 0x100
)

type FanotifyEvent struct {
	EventLen    uint32
	Vers        uint8
	Reserved    uint8
	Metadata    uint16
	Mask        uint64
	Fd          int32
	Pid         int32
	_           [4]byte // padding
}

// FanotifyMonitor monitors file system events using fanotify
type FanotifyMonitor struct {
	fd      int
	targets []string
}

// NewFanotifyMonitor creates a new fanotify monitor for targets
func NewFanotifyMonitor(targets []string) *FanotifyMonitor {
	return &FanotifyMonitor{
		fd:      -1,
		targets: targets,
	}
}

// Init initializes the fanotify monitor
func (fm *FanotifyMonitor) Init() error {
	// Initialize fanotify
	fd, err := unix.FanotifyInit(FAN_CLASS_NOTIF|FAN_UNLIMITED_QUEUE|FAN_UNLIMITED_MARKS, unix.O_CLOEXEC)
	if err != nil {
		return fmt.Errorf("fanotify_init failed: %w", err)
	}
	fm.fd = fd

	// Mark targets for monitoring
	for _, target := range fm.targets {
		err = unix.FanotifyMark(
			fd,
			FAN_MARK_ADD|FAN_MARK_FILESYSTEM,
			FAN_OPEN|FAN_CLOSE_WRITE|FAN_ACCESS|FAN_OPEN_EXEC,
			unix.AT_FDCWD,
			target,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: fanotify_mark for %s: %v\n", target, err)
			// Continue with other targets even if one fails
		}
	}

	return nil
}

// Close closes the fanotify monitor
func (fm *FanotifyMonitor) Close() error {
	if fm.fd >= 0 {
		return unix.Close(fm.fd)
	}
	return nil
}

// Start begins monitoring file system events and sends AuditEvents to the aggregator
func (fm *FanotifyMonitor) Start(eventsChan chan<- AuditEvent) error {
	if fm.fd < 0 {
		return fmt.Errorf("fanotify monitor not initialized")
	}

	buf := make([]byte, 4096)

	for {
		// Read events
		n, err := unix.Read(fm.fd, buf)
		if err != nil {
			return fmt.Errorf("read fanotify events: %w", err)
		}

		if n <= 0 {
			continue
		}

		// Parse events
		offset := 0
		for offset < n {
			if offset+unsafe.Sizeof(FanotifyEvent{}) > n {
				break
			}

			// Parse event header
			eventData := buf[offset : offset+int(unsafe.Sizeof(FanotifyEvent{}))]
			event := (*FanotifyEvent)(unsafe.Pointer(&eventData[0]))

			// Get file descriptor info
			fdPath := fmt.Sprintf("/proc/self/fd/%d", event.Fd)
			filePath, err := os.Readlink(fdPath)
			if err != nil {
				// Skip if we can't read the file path
				unix.Close(int(event.Fd))
				offset += int(event.EventLen)
				continue
			}

			// Get process name
			procPath := fmt.Sprintf("/proc/%d/comm", event.Pid)
			procNameBytes, err := os.ReadFile(procPath)
			var procName string
			if err == nil {
				procName = strings.TrimSpace(string(procNameBytes))
			} else {
				procName = fmt.Sprintf("pid_%d", event.Pid)
			}

			// Determine event type
			var eventType string
			switch {
			case event.Mask&FAN_OPEN != 0:
				eventType = "open"
			case event.Mask&FAN_CLOSE_WRITE != 0:
				eventType = "close_write"
			case event.Mask&FAN_ACCESS != 0:
				eventType = "read"
			case event.Mask&FAN_OPEN_EXEC != 0:
				eventType = "execve"
			default:
				eventType = "file_op"
			}

			// Create audit event
			auditEvent := AuditEvent{
				Timestamp:   time.Now(),
				UID:         fmt.Sprintf("%d", event.Pid),
				ProcessName: procName,
				FilePath:    filePath,
				EventType:   eventType,
				Success:     true,
			}

			// Send to aggregator
			select {
			case eventsChan <- auditEvent:
			default:
				// Channel full, drop event
			}

			// Close the file descriptor
			unix.Close(int(event.Fd))

			offset += int(event.EventLen)
		}
	}
}
