package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// Command-line flags
	monitorPaths := flag.String("paths", "/home,/etc,/var,/tmp,/opt", "Comma-separated paths to monitor")
	windowSize := flag.Duration("window", 10*time.Second, "Aggregation window size")
	baselineWindows := flag.Int("baseline", 30, "Number of windows for baseline learning")
	zscoreThreshold := flag.Float64("threshold", 2.5, "Z-score threshold for anomaly detection")
	verbose := flag.Bool("verbose", false, "Enable verbose output")

	flag.Parse()

	// Check for root privileges
	if os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "ERROR: fanotify requires root privileges (use sudo)\n")
		os.Exit(1)
	}

	// Parse monitor paths
	paths := strings.Split(*monitorPaths, ",")
	for i, p := range paths {
		paths[i] = strings.TrimSpace(p)
	}

	fmt.Printf("=== Linux Anomaly Detection Agent (fanotify) ===\n")
	fmt.Printf("Monitor paths: %s\n", strings.Join(paths, ", "))
	fmt.Printf("Window size: %v\n", *windowSize)
	fmt.Printf("Baseline windows: %d\n", *baselineWindows)
	fmt.Printf("Z-score threshold: %.1f\n", *zscoreThreshold)
	fmt.Printf("Verbose: %v\n", *verbose)
	fmt.Printf("==================================================\n\n")

	// Create fanotify monitor
	monitor := NewFanotifyMonitor(paths)
	if err := monitor.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to initialize fanotify: %v\n", err)
		os.Exit(1)
	}
	defer monitor.Close()

	// Create aggregator
	aggregator := NewAggregator(*windowSize)

	alertsChan := make(chan AnomalyAlert, 100)
	detector := NewAnomalyDetector(
		aggregator.GetFeaturesChan(),
		alertsChan,
		*baselineWindows,
		*zscoreThreshold,
	)

	// Start goroutines for components
	go aggregator.Start()
	go detector.Start()
	go printAlerts(alertsChan)

	// Start monitoring in a separate goroutine
	go func() {
		err := monitor.Start(aggregator.GetEventsChan())
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Monitor failed: %v\n", err)
			os.Exit(1)
		}
	}()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("[INFO] Agent started. Press Ctrl+C to stop.")
	fmt.Println("[INFO] Monitoring file system events...")
	fmt.Println("[INFO] Learning baseline behavior...")

	// Wait for shutdown signal
	sig := <-sigChan
	fmt.Printf("\n[INFO] Received signal %v, shutting down...\n", sig)

	monitor.Close()

	// Brief wait for cleanup
	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
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
