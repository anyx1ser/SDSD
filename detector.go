package main

import (
	"fmt"
	"math"
	"time"
)

// AnomalyDetector implements statistical anomaly detection
type AnomalyDetector struct {
	baselineWindowCount int           // Number of windows to use for baseline
	zscoreThreshold     float64       // Z-score threshold for anomaly detection (default 2.5)
	stats               *WindowStats
	windowsProcessed    int
	featuresChan        <-chan FeatureVector
	alertsChan          chan<- AnomalyAlert
}

// NewAnomalyDetector creates a new detector with specified parameters
func NewAnomalyDetector(
	featuresChan <-chan FeatureVector,
	alertsChan chan<- AnomalyAlert,
	baselineWindowCount int,
	zscoreThreshold float64,
) *AnomalyDetector {
	return &AnomalyDetector{
		baselineWindowCount: baselineWindowCount,
		zscoreThreshold:     zscoreThreshold,
		stats: &WindowStats{
			FileAccessCount:  make([]float64, 0),
			UniqueFileCount:  make([]float64, 0),
			ReadCount:        make([]float64, 0),
			ExecCount:        make([]float64, 0),
		},
		windowsProcessed: 0,
		featuresChan:     featuresChan,
		alertsChan:       alertsChan,
	}
}

// Start begins the anomaly detection process
// Should be run in a goroutine
func (ad *AnomalyDetector) Start() {
	for features := range ad.featuresChan {
		ad.processWindow(features)
	}
}

// processWindow analyzes a feature vector for anomalies
func (ad *AnomalyDetector) processWindow(features FeatureVector) {
	ad.windowsProcessed++

	// During baseline phase, collect statistics
	if ad.windowsProcessed <= ad.baselineWindowCount {
		ad.collectBaseline(features)

		// Once we have enough baseline data, compute statistics
		if ad.windowsProcessed == ad.baselineWindowCount {
			ad.computeBaselineStats()
			fmt.Printf("[INFO] Baseline learning complete after %d windows\n", ad.baselineWindowCount)
		}
		return
	}

	// Compute anomaly score based on collected statistics
	score := ad.computeAnomalyScore(features)
	features.AnomalyScore = score

	// Check if this is an anomaly
	if score > ad.zscoreThreshold {
		features.IsAnomaly = true
		ad.emitAlert(features, score)
	}
}

// collectBaseline stores feature values during the baseline learning phase
func (ad *AnomalyDetector) collectBaseline(features FeatureVector) {
	ad.stats.FileAccessCount = append(ad.stats.FileAccessCount, float64(features.FileAccessCount))
	ad.stats.UniqueFileCount = append(ad.stats.UniqueFileCount, float64(features.UniqueFileCount))
	ad.stats.ReadCount = append(ad.stats.ReadCount, float64(features.ReadCount))
	ad.stats.ExecCount = append(ad.stats.ExecCount, float64(features.ExecCount))
}

// computeBaselineStats calculates mean and standard deviation for all features
func (ad *AnomalyDetector) computeBaselineStats() {
	ad.stats.FileAccessMean, ad.stats.FileAccessStdDev = ad.calculateMeanStdDev(ad.stats.FileAccessCount)
	ad.stats.UniqueFileMean, ad.stats.UniqueFileStdDev = ad.calculateMeanStdDev(ad.stats.UniqueFileCount)
	ad.stats.ReadCountMean, ad.stats.ReadCountStdDev = ad.calculateMeanStdDev(ad.stats.ReadCount)
	ad.stats.ExecCountMean, ad.stats.ExecCountStdDev = ad.calculateMeanStdDev(ad.stats.ExecCount)

	fmt.Printf("[STATS] FileAccess: mean=%.1f, stddev=%.1f\n",
		ad.stats.FileAccessMean, ad.stats.FileAccessStdDev)
	fmt.Printf("[STATS] UniqueFile: mean=%.1f, stddev=%.1f\n",
		ad.stats.UniqueFileMean, ad.stats.UniqueFileStdDev)
	fmt.Printf("[STATS] ReadCount: mean=%.1f, stddev=%.1f\n",
		ad.stats.ReadCountMean, ad.stats.ReadCountStdDev)
	fmt.Printf("[STATS] ExecCount: mean=%.1f, stddev=%.1f\n",
		ad.stats.ExecCountMean, ad.stats.ExecCountStdDev)
}

// computeAnomalyScore calculates the maximum z-score across all features
// Returns the highest z-score observed
func (ad *AnomalyDetector) computeAnomalyScore(features FeatureVector) float64 {
	// Prevent division by zero - if stddev is 0, baseline has no variance
	if ad.stats.FileAccessStdDev == 0 &&
		ad.stats.UniqueFileStdDev == 0 &&
		ad.stats.ReadCountStdDev == 0 &&
		ad.stats.ExecCountStdDev == 0 {
		return 0.0
	}

	maxZScore := 0.0

	// Compute z-scores for each feature
	zscores := make([]float64, 0)

	// File access count z-score
	if ad.stats.FileAccessStdDev > 0 {
		z := math.Abs((float64(features.FileAccessCount) - ad.stats.FileAccessMean) / ad.stats.FileAccessStdDev)
		zscores = append(zscores, z)
	}

	// Unique file count z-score
	if ad.stats.UniqueFileStdDev > 0 {
		z := math.Abs((float64(features.UniqueFileCount) - ad.stats.UniqueFileMean) / ad.stats.UniqueFileStdDev)
		zscores = append(zscores, z)
	}

	// Read count z-score
	if ad.stats.ReadCountStdDev > 0 {
		z := math.Abs((float64(features.ReadCount) - ad.stats.ReadCountMean) / ad.stats.ReadCountStdDev)
		zscores = append(zscores, z)
	}

	// Exec count z-score
	if ad.stats.ExecCountStdDev > 0 {
		z := math.Abs((float64(features.ExecCount) - ad.stats.ExecCountMean) / ad.stats.ExecCountStdDev)
		zscores = append(zscores, z)
	}

	// Return the maximum z-score
	for _, z := range zscores {
		if z > maxZScore {
			maxZScore = z
		}
	}

	return maxZScore
}

// calculateMeanStdDev calculates mean and standard deviation for a slice
func (ad *AnomalyDetector) calculateMeanStdDev(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0.0, 0.0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate standard deviation
	varSum := 0.0
	for _, v := range values {
		diff := v - mean
		varSum += diff * diff
	}
	variance := varSum / float64(len(values))
	stdDev := math.Sqrt(variance)

	return mean, stdDev
}

// emitAlert sends an anomaly alert
func (ad *AnomalyDetector) emitAlert(features FeatureVector, score float64) {
	reason := ad.generateAlertReason(features)

	alert := AnomalyAlert{
		Time:        time.Now(),
		WindowIndex: features.WindowIndex,
		Score:       score,
		Reason:      reason,
		Features:    features,
	}

	select {
	case ad.alertsChan <- alert:
	default:
		// Channel full
	}
}

// generateAlertReason creates a human-readable description of what triggered the alert
func (ad *AnomalyDetector) generateAlertReason(features FeatureVector) string {
	reasons := []string{}

	// Check which features exceeded thresholds
	if ad.stats.FileAccessStdDev > 0 {
		z := math.Abs((float64(features.FileAccessCount) - ad.stats.FileAccessMean) / ad.stats.FileAccessStdDev)
		if z > ad.zscoreThreshold {
			reasons = append(reasons, fmt.Sprintf("High file access rate (z=%.2f, count=%d)",
				z, features.FileAccessCount))
		}
	}

	if ad.stats.UniqueFileStdDev > 0 {
		z := math.Abs((float64(features.UniqueFileCount) - ad.stats.UniqueFileMean) / ad.stats.UniqueFileStdDev)
		if z > ad.zscoreThreshold {
			reasons = append(reasons, fmt.Sprintf("Unusual unique file count (z=%.2f, count=%d)",
				z, features.UniqueFileCount))
		}
	}

	if ad.stats.ReadCountStdDev > 0 {
		z := math.Abs((float64(features.ReadCount) - ad.stats.ReadCountMean) / ad.stats.ReadCountStdDev)
		if z > ad.zscoreThreshold {
			reasons = append(reasons, fmt.Sprintf("High read operation count (z=%.2f, count=%d)",
				z, features.ReadCount))
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "Anomaly detected")
	}

	return reasons[0]
}
