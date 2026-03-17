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
	// ── Command-line flags ────────────────────────────────────────────────────
	monitorPaths := flag.String("paths", "/home,/etc", "Comma-separated paths to monitor")
	windowSize := flag.Duration("window", 10*time.Second, "Aggregation window size")
	windowStep := flag.Duration("step", 2*time.Second, "Sliding-window step interval")
	baselineWindows := flag.Int("baseline", 60, "Windows to collect per UID before training the Isolation Forest")
	dbPath := flag.String("db", "sdsd_baselines.db", "SQLite database file for persisting user baselines")
	contamination := flag.Float64("contamination", 0.1, "Expected fraction of anomalies in training data (IsolationForest)")
	numTrees := flag.Int("estimators", 100, "Number of isolation trees (IsolationForest n_estimators)")
	sampleSize := flag.Int("sample-size", 256, "Sub-sample size per tree (IsolationForest max_samples)")
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

	fmt.Printf("=== SDSD — Isolation Forest Anomaly Detection Agent ===\n")
	fmt.Printf("Monitor paths  : %s\n", strings.Join(paths, ", "))
	fmt.Printf("Window size    : %v\n", *windowSize)
	fmt.Printf("Window step    : %v\n", *windowStep)
	fmt.Printf("Baseline windows/UID : %d\n", *baselineWindows)
	fmt.Printf("ML model       : IsolationForest(n_estimators=%d, max_samples=%d, contamination=%.2f)\n",
		*numTrees, *sampleSize, *contamination)
	fmt.Printf("Database       : %s\n", *dbPath)
	fmt.Printf("Verbose        : %v\n", *verbose)
	fmt.Printf("=======================================================\n\n")
	_ = verbose // used implicitly by log-level settings if extended later

	// ── Open SQLite database ─────────────────────────────────────────────────
	db, err := NewDatabase(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	uids, _ := db.ListBaselineUIDs()
	if len(uids) > 0 {
		fmt.Printf("[DB] Existing baselines found for UIDs: %s\n", strings.Join(uids, ", "))
	}

	// ── Build pipeline  ──────────────────────────────────────────────────────
	monitor := NewFanotifyMonitor(paths)
	if err := monitor.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to initialize fanotify: %v\n", err)
		os.Exit(1)
	}
	defer monitor.Close()

	aggregator := NewAggregator(*windowSize, *windowStep)
	alertsChan := make(chan AnomalyAlert, 100)

	detector := NewAnomalyDetector(
		aggregator.GetFeaturesChan(),
		alertsChan,
		db,
		*baselineWindows,
		*numTrees,
		*sampleSize,
		*contamination,
	)

	// ── Start goroutines ─────────────────────────────────────────────────────
	go aggregator.Start()
	go detector.Start()
	go printAlerts(alertsChan)

	go func() {
		if err := monitor.Start(aggregator.GetEventsChan()); err != nil {
			fmt.Fprintf(os.Stderr, "[ERROR] Monitor failed: %v\n", err)
			os.Exit(1)
		}
	}()

	// ── Graceful shutdown ────────────────────────────────────────────────────
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("[INFO] Agent started. Press Ctrl+C to stop.")
	fmt.Println("[INFO] Monitoring file system events...")
	fmt.Println("[INFO] Collecting per-user baselines for Isolation Forest training...")

	sig := <-sigChan
	fmt.Printf("\n[INFO] Received signal %v, shutting down...\n", sig)

	monitor.Close()
	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
}

// printAlerts outputs detected anomalies to stdout.
func printAlerts(alertsChan <-chan AnomalyAlert) {
	for alert := range alertsChan {
		fmt.Printf("[ALERT] timestamp=%s user=%-6s score=%.4f reason=%s\n",
			alert.Time.Format("2006-01-02 15:04:05"),
			alert.UID,
			alert.Score,
			alert.Reason,
		)
		fmt.Printf("        accesses=%d reads=%d writes=%d unique_files=%d unique_dirs=%d rate=%.2f/s burst=%d archive_pattern=%v\n",
			alert.Features.TotalFileAccesses,
			alert.Features.ReadOperations,
			alert.Features.WriteOperations,
			alert.Features.UniqueFilesAccessed,
			alert.Features.UniqueDirectoriesAccessed,
			alert.Features.FileAccessRate,
			alert.Features.MaxEventsInShortInterval,
			alert.Features.ArchiveLikeAccessPattern,
		)
	}
}
