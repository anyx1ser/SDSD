package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AuditParser parses audit log lines into structured events
type AuditParser struct {
	// Regex patterns for different audit event types
	syscallPattern *regexp.Regexp
	msgPattern     *regexp.Regexp
	execPattern    *regexp.Regexp
}

// NewAuditParser creates a new parser for audit logs
func NewAuditParser() *AuditParser {
	return &AuditParser{
		// Pattern for SYSCALL events: type=SYSCALL msg=audit(timestamp.seq): arch=...
		syscallPattern: regexp.MustCompile(`type=SYSCALL.*msg=audit\((\d+)\.(\d+)\):.*syscall=(\d+).*uid=(\d+).*exe="([^"]*)".*`),
		// Pattern for EXECVE events: type=EXECVE msg=audit(...): argc=...
		execPattern: regexp.MustCompile(`type=EXECVE.*msg=audit\((\d+)\.(\d+)\):.*argc=(\d+)`),
		// Generic message pattern
		msgPattern: regexp.MustCompile(`msg=audit\((\d+)\.(\d+)\):`),
	}
}

// ParseEvent parses a raw audit log line into an AuditEvent
// Returns nil if the line cannot be parsed or is not relevant
func (ap *AuditParser) ParseEvent(line string) *AuditEvent {
	// Skip empty lines
	if strings.TrimSpace(line) == "" {
		return nil
	}

	// Extract timestamp and sequence number
	msgMatches := ap.msgPattern.FindStringSubmatch(line)
	if len(msgMatches) < 3 {
		return nil
	}

	timestampSec, err := strconv.ParseInt(msgMatches[1], 10, 64)
	if err != nil {
		return nil
	}

	timestamp := time.Unix(timestampSec, 0)

	// Parse SYSCALL events
	event := ap.parseSyscallEvent(line, timestamp)
	if event != nil {
		return event
	}

	// Parse EXECVE events
	event = ap.parseExecveEvent(line, timestamp)
	if event != nil {
		return event
	}

	return nil
}

// parseSyscallEvent extracts syscall event information
func (ap *AuditParser) parseSyscallEvent(line string, timestamp time.Time) *AuditEvent {
	// Skip if this is an EXECVE line (handled separately)
	if strings.Contains(line, "type=EXECVE") {
		return nil
	}

	// Extract uid
	uidMatch := regexp.MustCompile(`uid=(\d+)`).FindStringSubmatch(line)
	if len(uidMatch) < 2 {
		return nil
	}

	// Extract exe/process name
	exeMatch := regexp.MustCompile(`exe="([^"]*)"`).FindStringSubmatch(line)
	if len(exeMatch) < 2 {
		return nil
	}

	processName := extractBaseName(exeMatch[1])

	// Determine syscall type and relevant file path
	syscallType := ""
	filePath := ""

	// Look for open/openat syscalls (1 = write, 2 = open, 257 = openat, etc.)
	if strings.Contains(line, "syscall=2") || strings.Contains(line, "syscall=257") {
		syscallType = "open"
		filePath = extractFilePath(line)
	} else if strings.Contains(line, "syscall=0") {
		syscallType = "read"
		filePath = extractFilePath(line)
	}

	// Only process relevant event types
	if syscallType == "" {
		return nil
	}

	success := !strings.Contains(line, "exit=-")

	return &AuditEvent{
		Timestamp:   timestamp,
		UID:         uidMatch[1],
		ProcessName: processName,
		FilePath:    filePath,
		EventType:   syscallType,
		Success:     success,
	}
}

// parseExecveEvent extracts execve event information
func (ap *AuditParser) parseExecveEvent(line string, timestamp time.Time) *AuditEvent {
	if !strings.Contains(line, "type=EXECVE") {
		return nil
	}

	// Extract uid
	uidMatch := regexp.MustCompile(`uid=(\d+)`).FindStringSubmatch(line)
	if len(uidMatch) < 2 {
		return nil
	}

	// Extract program name from the first argv
	// Format: argc=X a0="program" a1="arg1" ...
	argvMatch := regexp.MustCompile(`a0="([^"]*)"`).FindStringSubmatch(line)
	if len(argvMatch) < 2 {
		return nil
	}

	processName := extractBaseName(argvMatch[1])

	return &AuditEvent{
		Timestamp:   timestamp,
		UID:         uidMatch[1],
		ProcessName: processName,
		FilePath:    argvMatch[1],
		EventType:   "execve",
		Success:     true,
	}
}

// extractFilePath extracts file path from audit line
func extractFilePath(line string) string {
	// Look for name="..." pattern which typically contains the file path
	nameMatch := regexp.MustCompile(`name="([^"]*)"`).FindStringSubmatch(line)
	if len(nameMatch) > 1 {
		return nameMatch[1]
	}

	// Try to find path in other formats
	pathMatch := regexp.MustCompile(`path="([^"]*)"`).FindStringSubmatch(line)
	if len(pathMatch) > 1 {
		return pathMatch[1]
	}

	return ""
}

// extractBaseName extracts just the filename/binary name from a path
func extractBaseName(path string) string {
	if path == "" {
		return "unknown"
	}

	parts := strings.Split(path, "/")
	baseName := parts[len(parts)-1]

	if baseName == "" {
		return path
	}

	return baseName
}
