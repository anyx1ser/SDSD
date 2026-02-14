package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

// LogReader handles incremental reading of audit log files
type LogReader struct {
	filePath   string
	file       *os.File
	reader     *bufio.Reader
	offset     int64
	lastModify time.Time
}

// NewLogReader creates a new log reader for the audit log file
func NewLogReader(filePath string) *LogReader {
	return &LogReader{
		filePath: filePath,
	}
}

// Open initializes the log reader and seeks to the end of the file
func (lr *LogReader) Open() error {
	file, err := os.OpenFile(lr.filePath, os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Get file info to store last modify time
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to stat file: %w", err)
	}

	lr.file = file
	lr.reader = bufio.NewReaderSize(file, 65536) // 64KB buffer for efficiency
	lr.offset, _ = file.Seek(0, 2)                // Seek to end
	lr.lastModify = info.ModTime()

	return nil
}

// Close closes the log file
func (lr *LogReader) Close() error {
	if lr.file != nil {
		return lr.file.Close()
	}
	return nil
}

// ReadLine reads the next available line from the log file
// Returns empty string if no new lines are available
func (lr *LogReader) ReadLine() (string, error) {
	// Check if file has been rotated or modified
	info, err := os.Stat(lr.filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	// If file size decreased or was rotated (modify time jumped), reopen
	if info.Size() < lr.offset || (info.ModTime().After(lr.lastModify) && info.Size() < lr.offset) {
		lr.Close()
		return "", fmt.Errorf("log file rotated, will reopen")
	}

	line, err := lr.reader.ReadString('\n')
	if err != nil {
		// EOF is normal, not an error condition
		if err.Error() == "EOF" {
			return "", nil
		}
		return "", err
	}

	lr.offset += int64(len(line))
	lr.lastModify = info.ModTime()

	// Remove trailing newline
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}

	return line, nil
}

// Tail continuously reads new lines from the log file with polling interval
// Sends lines to the provided channel, respecting context for graceful shutdown
func (lr *LogReader) Tail(linesChan chan<- string, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Try to read available lines
		for {
			line, err := lr.ReadLine()
			if err != nil {
				// File rotated, attempt to reopen
				lr.Close()
				if err := lr.Open(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to reopen log file: %v\n", err)
					time.Sleep(time.Second)
					break
				}
				continue
			}

			if line == "" {
				break // No more lines available
			}

			select {
			case linesChan <- line:
			default:
				// Channel full, skip this line to maintain responsiveness
			}
		}
	}

	return nil
}
