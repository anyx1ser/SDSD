package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Command-line flags
	logPath := flag.String("log", "/var/log/audit/audit.log", "Path to audit log file")
	windowSize := flag.Duration("window", 10*time.Second, "Aggregation window size")
	baselineWindows := flag.Int("baseline", 30, "Number of windows for baseline learning")
	zscoreThreshold := flag.Float64("threshold", 2.5, "Z-score threshold for anomaly detection")
	pollInterval := flag.Duration("poll", 500*time.Millisecond, "Poll interval for log file")
	verbose := flag.Bool("verbose", false, "Enable verbose output")

	flag.Parse()

	fmt.Printf("=== Linux Anomaly Detection Agent ===\n")
	fmt.Printf("Log file: %s\n", *logPath)
	fmt.Printf("Window size: %v\n", *windowSize)
	fmt.Printf("Baseline windows: %d\n", *baselineWindows)
	fmt.Printf("Z-score threshold: %.1f\n", *zscoreThreshold)
	fmt.Printf("Poll interval: %v\n", *pollInterval)
	fmt.Printf("=====================================\n\n")

	// Verify audit log exists and is readable
	if _, err := os.Stat(*logPath); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Cannot access audit log: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure auditd is running and you have sudo privileges\n")
		os.Exit(1)
	}

	// Create components
	logReader := NewLogReader(*logPath)
	if err := logReader.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to open log: %v\n", err)
		os.Exit(1)
	}
	defer logReader.Close()

	parser := NewAuditParser()
	aggregator := NewAggregator(*windowSize)

	alertsChan := make(chan AnomalyAlert, 100)
	detector := NewAnomalyDetector(
		aggregator.GetFeaturesChan(),
		alertsChan,
		*baselineWindows,
		*zscoreThreshold,
	)

	// Start goroutines for each component
	linesChan := make(chan string, 1000)

	go logReader.Tail(linesChan, *pollInterval)
	go aggregator.Start()
	go detector.Start()
	go processLines(linesChan, parser, aggregator, *verbose)
	go printAlerts(alertsChan)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("[INFO] Agent started. Press Ctrl+C to stop.")
	fmt.Println("[INFO] Learning baseline behavior...")

	// Wait for shutdown signal
	sig := <-sigChan
	fmt.Printf("\n[INFO] Received signal %v, shutting down...\n", sig)

	logReader.Close()
	close(linesChan)

	// Brief wait for cleanup
	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
}

// processLines reads audit log lines and sends them to the parser
func processLines(linesChan <-chan string, parser *AuditParser, aggregator *Aggregator, verbose bool) {
	eventsChan := aggregator.GetEventsChan()
	parseErrors := 0

	for line := range linesChan {
		event := parser.ParseEvent(line)
		if event != nil {
			select {
			case eventsChan <- *event:
				if verbose {
					fmt.Printf("[PARSE] %s: %s on %s\n", event.ProcessName, event.EventType, event.FilePath)
				}
			default:
				// Channel full
			}
		} else {
			parseErrors++
			if verbose && parseErrors%1000 == 0 {
				fmt.Printf("[WARN] Unable to parse events (last 1000 lines)\n")
			}
		}
	}
}

// printAlerts outputs detected anomalies in a formatted manner
func printAlerts(alertsChan <-chan AnomalyAlert) {
	for alert := range alertsChan {
		fmt.Printf("[ALERT] time=%s window=%d score=%.2f reason=%s\n",
			alert.Time.Format("2006-01-02 15:04:05"),
			alert.WindowIndex,
			alert.Score,
			alert.Reason,
		)
		fmt.Printf("        FileAccessCount=%d UniqueFiles=%d ReadOps=%d ExecOps=%d\n",
			alert.Features.FileAccessCount,
			alert.Features.UniqueFileCount,
			alert.Features.ReadCount,
			alert.Features.ExecCount,
		)
	}
}
