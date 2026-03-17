package main

import (
	"math"
	"testing"
	"time"
)

func TestComputeMeanVarianceAndAggregateZScore(t *testing.T) {
	samples := [][]float64{
		{1, 2, 3},
		{2, 3, 4},
		{3, 4, 5},
	}

	mean, variance := computeMeanVariance(samples)
	if len(mean) != 3 || len(variance) != 3 {
		t.Fatalf("unexpected dimensions mean=%d variance=%d", len(mean), len(variance))
	}

	if math.Abs(mean[0]-2.0) > 1e-9 || math.Abs(mean[1]-3.0) > 1e-9 || math.Abs(mean[2]-4.0) > 1e-9 {
		t.Fatalf("unexpected mean=%v", mean)
	}
	if math.Abs(variance[0]-1.0) > 1e-9 || math.Abs(variance[1]-1.0) > 1e-9 || math.Abs(variance[2]-1.0) > 1e-9 {
		t.Fatalf("unexpected variance=%v", variance)
	}

	std := buildStdDev(variance)
	zNearMean := aggregateZScore([]float64{2, 3, 4}, mean, std)
	zFar := aggregateZScore([]float64{20, 30, 40}, mean, std)
	if zNearMean > 1e-9 {
		t.Fatalf("zNearMean=%f, want ~0", zNearMean)
	}
	if zFar <= zNearMean {
		t.Fatalf("zFar=%f should be greater than zNearMean=%f", zFar, zNearMean)
	}
}

func TestReasonTagsCoverage(t *testing.T) {
	fv := FeatureVector{
		TotalFileAccesses:         250,
		ReadOperations:            170,
		UniqueDirectoriesAccessed: 30,
		DirectoryAccessEntropy:    3.8,
		ArchiveLikeAccessPattern:  true,
		TransferProcessDetected:   true,
		FileExtensionEntropy:      3.8,
		FilenameEntropy:           3.6,
		MaxEventsInShortInterval:  45,
		FileAccessRate:            21.0,
	}

	tags := reasonTags(fv, 7.0)

	want := []string{
		"mass file access",
		"recursive directory traversal",
		"archive creation detected",
		"abnormal entropy of accessed files",
		"burst filesystem activity",
		"suspicious transfer process activity",
		"strong baseline deviation",
	}

	for _, w := range want {
		if !containsString(tags, w) {
			t.Fatalf("missing expected tag %q in %v", w, tags)
		}
	}
}

func TestColdStartReasonTagsPrivilegedTransfer(t *testing.T) {
	fv := FeatureVector{
		UID:                      "0",
		TransferProcessDetected:  true,
		ReadOperations:           35,
		FileAccessRate:           12.0,
		MaxEventsInShortInterval: 24,
	}

	tags := coldStartReasonTags(fv)
	if !containsString(tags, "suspicious transfer process activity during baseline") {
		t.Fatalf("missing transfer cold-start tag: %v", tags)
	}
	if !containsString(tags, "privileged user burst activity during baseline") {
		t.Fatalf("missing privileged cold-start tag: %v", tags)
	}
}

func TestPercentileBoundaries(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}

	if got := percentile(sorted, 0); got != 1 {
		t.Fatalf("percentile p=0 got=%f want=1", got)
	}
	if got := percentile(sorted, 1); got != 5 {
		t.Fatalf("percentile p=1 got=%f want=5", got)
	}
	if got := percentile(sorted, 0.5); got < 2 || got > 4 {
		t.Fatalf("percentile p=0.5 got=%f want in [2,4]", got)
	}
}

func TestReasonTagsEntropyRequiresSupportingSignal(t *testing.T) {
	fvEntropyOnlyLowActivity := FeatureVector{
		FileExtensionEntropy:      3.8,
		FilenameEntropy:           3.6,
		TotalFileAccesses:         30,
		UniqueDirectoriesAccessed: 4,
	}
	tags := reasonTags(fvEntropyOnlyLowActivity, 2.0)
	if containsString(tags, "abnormal entropy of accessed files") {
		t.Fatalf("unexpected entropy tag for low-activity window: %v", tags)
	}

	fvEntropyHighActivity := FeatureVector{
		FileExtensionEntropy:      3.8,
		FilenameEntropy:           3.6,
		TotalFileAccesses:         180,
		UniqueDirectoriesAccessed: 25,
	}
	tags2 := reasonTags(fvEntropyHighActivity, 2.0)
	if !containsString(tags2, "abnormal entropy of accessed files") {
		t.Fatalf("expected entropy tag for high-activity window, got: %v", tags2)
	}
}

func TestShouldSuppressAlertEntropyCooldown(t *testing.T) {
	now := time.Unix(2000, 0)
	last := now.Add(-5 * time.Minute)
	if !shouldSuppressAlert(
		"abnormal entropy of accessed files",
		true,
		last,
		"abnormal entropy of accessed files",
		now,
	) {
		t.Fatal("expected entropy-only alert to be suppressed within cooldown")
	}

	lastOld := now.Add(-11 * time.Minute)
	if shouldSuppressAlert(
		"abnormal entropy of accessed files",
		true,
		lastOld,
		"abnormal entropy of accessed files",
		now,
	) {
		t.Fatal("did not expect entropy-only alert suppression after cooldown")
	}
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
