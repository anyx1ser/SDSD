package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type detectorPhase int

const (
	phaseCollecting detectorPhase = iota
	phaseDetecting
)

type baselineStats struct {
	mean       []float64
	variance   []float64
	std        []float64
	zThreshold float64
}

type perUIDState struct {
	phase        detectorPhase
	trainingData [][]float64
	stats        baselineStats
	model        *IsolationForest
	lastAlertAt  time.Time
	lastReason   string
}

// AnomalyDetector learns per-UID normal behavior and scores live windows.
type AnomalyDetector struct {
	baselineWindowCount int
	numTrees            int
	sampleSize          int
	contamination       float64

	featuresChan <-chan FeatureVector
	alertsChan   chan<- AnomalyAlert
	db           *Database

	uids map[string]*perUIDState
}

func NewAnomalyDetector(
	featuresChan <-chan FeatureVector,
	alertsChan chan<- AnomalyAlert,
	db *Database,
	baselineWindowCount int,
	numTrees int,
	sampleSize int,
	contamination float64,
) *AnomalyDetector {
	if baselineWindowCount < 20 {
		baselineWindowCount = 20
	}
	return &AnomalyDetector{
		baselineWindowCount: baselineWindowCount,
		numTrees:            numTrees,
		sampleSize:          sampleSize,
		contamination:       contamination,
		featuresChan:        featuresChan,
		alertsChan:          alertsChan,
		db:                  db,
		uids:                make(map[string]*perUIDState),
	}
}

func (ad *AnomalyDetector) Start() {
	for fv := range ad.featuresChan {
		ad.processWindow(fv)
	}
}

func (ad *AnomalyDetector) processWindow(fv FeatureVector) {
	uid := fv.UID
	if _, ok := ad.uids[uid]; !ok {
		ad.uids[uid] = ad.loadOrInitUID(uid)
	}
	state := ad.uids[uid]

	sample := fv.ToFeatureArray()
	switch state.phase {
	case phaseCollecting:
		ad.maybeAlertDuringCollecting(uid, state, &fv)
		state.trainingData = append(state.trainingData, sample)
		n := len(state.trainingData)
		fmt.Printf("[INFO] Baseline uid=%-6s collecting %d/%d\n", uid, n, ad.baselineWindowCount)
		if n >= ad.baselineWindowCount {
			ad.trainAndPersist(uid, state)
		}
	case phaseDetecting:
		ad.scoreAndAlert(uid, state, &fv, sample)
	}

	if ad.db != nil {
		if err := ad.db.InsertFeatureVector(fv); err != nil {
			fmt.Printf("[WARN] DB insert feature vector: %v\n", err)
		}
	}
}

func (ad *AnomalyDetector) loadOrInitUID(uid string) *perUIDState {
	state := &perUIDState{phase: phaseCollecting}
	if ad.db == nil {
		return state
	}

	b, found, err := ad.db.LoadBaseline(uid)
	if err != nil {
		fmt.Printf("[WARN] DB load baseline uid=%s: %v\n", uid, err)
		return state
	}
	if !found {
		return state
	}

	var mean []float64
	var variance []float64
	if len(b.MeanJSON) > 0 {
		_ = json.Unmarshal(b.MeanJSON, &mean)
	}
	if len(b.VarianceJSON) > 0 {
		_ = json.Unmarshal(b.VarianceJSON, &variance)
	}
	std := buildStdDev(variance)

	var model *IsolationForest
	if len(b.ModelJSON) > 0 {
		model, err = UnmarshalIsolationForest(b.ModelJSON)
		if err != nil {
			fmt.Printf("[WARN] Corrupt IF model for uid=%s: %v\n", uid, err)
		}
	}

	if len(mean) > 0 && len(variance) == len(mean) {
		state.stats = baselineStats{
			mean:       mean,
			variance:   variance,
			std:        std,
			zThreshold: b.ZThreshold,
		}
		if state.stats.zThreshold <= 0 {
			state.stats.zThreshold = 3.5
		}
		state.model = model
		state.phase = phaseDetecting
		fmt.Printf("[INFO] Loaded baseline for uid=%-6s trained=%s n=%d z_threshold=%.2f\n",
			uid, b.TrainedAt.Format("2006-01-02 15:04:05"), b.NSamples, state.stats.zThreshold)
		return state
	}

	return &perUIDState{phase: phaseCollecting}
}

func (ad *AnomalyDetector) trainAndPersist(uid string, state *perUIDState) {
	mean, variance := computeMeanVariance(state.trainingData)
	if len(mean) == 0 {
		return
	}
	std := buildStdDev(variance)

	zScores := make([]float64, len(state.trainingData))
	for i, sample := range state.trainingData {
		zScores[i] = aggregateZScore(sample, mean, std)
	}
	sort.Float64s(zScores)
	zThreshold := percentile(zScores, 0.95)
	if zThreshold < 2.5 {
		zThreshold = 2.5
	}

	forest := NewIsolationForest(ad.numTrees, ad.sampleSize, ad.contamination)
	forest.Fit(state.trainingData)

	state.stats = baselineStats{
		mean:       mean,
		variance:   variance,
		std:        std,
		zThreshold: zThreshold,
	}
	state.model = forest
	state.phase = phaseDetecting

	fmt.Printf("[INFO] Baseline ready uid=%-6s z_threshold=%.3f if_threshold=%.4f samples=%d\n",
		uid, zThreshold, forest.Threshold, len(state.trainingData))

	if ad.db != nil {
		meanJSON, _ := json.Marshal(mean)
		varJSON, _ := json.Marshal(variance)
		modelJSON, err := forest.Marshal()
		if err != nil {
			fmt.Printf("[WARN] Serialise IF model uid=%s: %v\n", uid, err)
		} else {
			b := UserBaseline{
				UID:           uid,
				TrainedAt:     time.Now(),
				NSamples:      len(state.trainingData),
				Contamination: ad.contamination,
				ZThreshold:    zThreshold,
				MeanJSON:      meanJSON,
				VarianceJSON:  varJSON,
				ModelJSON:     modelJSON,
			}
			if err := ad.db.SaveBaseline(b); err != nil {
				fmt.Printf("[WARN] DB save baseline uid=%s: %v\n", uid, err)
			}
		}
	}

	state.trainingData = nil
}

func (ad *AnomalyDetector) scoreAndAlert(uid string, state *perUIDState, fv *FeatureVector, sample []float64) {
	zAgg := aggregateZScore(sample, state.stats.mean, state.stats.std)
	ifAnomaly := false
	ifScore := 0.0
	if state.model != nil {
		ifAnomaly, ifScore = state.model.IsAnomaly(sample)
	}

	normZ := zAgg / math.Max(state.stats.zThreshold, 1.0)
	if normZ > 2.0 {
		normZ = 2.0
	}
	finalScore := 0.65*normZ + 0.35*ifScore

	reasons := reasonTags(*fv, zAgg)
	isAnomaly := zAgg >= state.stats.zThreshold || (ifAnomaly && finalScore >= 0.65)
	if len(reasons) > 0 && finalScore >= 0.55 {
		isAnomaly = true
	}

	if isEntropyOnlyReason(reasons) && zAgg < state.stats.zThreshold*1.15 && !ifAnomaly {
		isAnomaly = false
	}

	fv.AnomalyScore = finalScore
	fv.IsAnomaly = isAnomaly
	if !isAnomaly {
		return
	}

	reason := "abnormal filesystem activity"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, ", ")
	}

	if shouldSuppressAlert(reason, isEntropyOnlyReason(reasons), state.lastAlertAt, state.lastReason, time.Now()) {
		return
	}

	alert := AnomalyAlert{
		Time:        time.Now(),
		WindowIndex: fv.WindowIndex,
		UID:         uid,
		Score:       finalScore,
		Reason:      reason,
		Features:    *fv,
	}
	state.lastAlertAt = alert.Time
	state.lastReason = reason

	select {
	case ad.alertsChan <- alert:
	default:
	}

	if ad.db != nil {
		if err := ad.db.InsertAlert(alert); err != nil {
			fmt.Printf("[WARN] DB insert alert: %v\n", err)
		}
	}
}

func (ad *AnomalyDetector) maybeAlertDuringCollecting(uid string, state *perUIDState, fv *FeatureVector) {
	reasons := coldStartReasonTags(*fv)
	if len(reasons) == 0 {
		return
	}

	reason := strings.Join(reasons, ", ")
	now := time.Now()
	if shouldSuppressAlert(reason, false, state.lastAlertAt, state.lastReason, now) {
		return
	}

	// Cold-start alerts have no trained z-score yet, so assign a fixed high score.
	fv.AnomalyScore = 0.8
	fv.IsAnomaly = true

	alert := AnomalyAlert{
		Time:        now,
		WindowIndex: fv.WindowIndex,
		UID:         uid,
		Score:       fv.AnomalyScore,
		Reason:      reason,
		Features:    *fv,
	}
	state.lastAlertAt = alert.Time
	state.lastReason = reason

	select {
	case ad.alertsChan <- alert:
	default:
	}

	if ad.db != nil {
		if err := ad.db.InsertAlert(alert); err != nil {
			fmt.Printf("[WARN] DB insert alert: %v\n", err)
		}
	}
}

func computeMeanVariance(samples [][]float64) ([]float64, []float64) {
	if len(samples) == 0 {
		return nil, nil
	}
	dims := len(samples[0])
	mean := make([]float64, dims)
	m2 := make([]float64, dims)

	n := 0.0
	for _, s := range samples {
		n++
		for i := 0; i < dims; i++ {
			delta := s[i] - mean[i]
			mean[i] += delta / n
			delta2 := s[i] - mean[i]
			m2[i] += delta * delta2
		}
	}

	variance := make([]float64, dims)
	if n <= 1 {
		return mean, variance
	}
	for i := 0; i < dims; i++ {
		variance[i] = m2[i] / (n - 1.0)
	}
	return mean, variance
}

func buildStdDev(variance []float64) []float64 {
	std := make([]float64, len(variance))
	for i := range variance {
		if variance[i] <= 1e-9 {
			std[i] = 1.0
		} else {
			std[i] = math.Sqrt(variance[i])
		}
	}
	return std
}

func aggregateZScore(sample, mean, std []float64) float64 {
	if len(sample) == 0 || len(mean) != len(sample) || len(std) != len(sample) {
		return 0
	}
	total := 0.0
	for i := range sample {
		z := math.Abs(sample[i]-mean[i]) / std[i]
		if z > 10 {
			z = 10
		}
		total += z
	}
	return total / float64(len(sample))
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Floor(p * float64(len(sorted)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func reasonTags(fv FeatureVector, zAgg float64) []string {
	reasons := make([]string, 0, 6)
	if fv.TotalFileAccesses >= 200 || fv.ReadOperations >= 150 {
		reasons = append(reasons, "mass file access")
	}
	if fv.UniqueDirectoriesAccessed >= 25 && fv.DirectoryAccessEntropy >= 3.5 {
		reasons = append(reasons, "recursive directory traversal")
	}
	if fv.ArchiveLikeAccessPattern || (fv.ArchiveProcessDetected && fv.ArchiveOutputCreated) {
		reasons = append(reasons, "archive creation detected")
	}
	if (fv.FileExtensionEntropy >= 3.6 || fv.FilenameEntropy >= 3.4) &&
		(fv.TotalFileAccesses >= 120 || fv.UniqueDirectoriesAccessed >= 20 || zAgg >= 5.5) {
		reasons = append(reasons, "abnormal entropy of accessed files")
	}
	if fv.MaxEventsInShortInterval >= 40 || fv.FileAccessRate >= 20 {
		reasons = append(reasons, "burst filesystem activity")
	}
	if fv.TransferProcessDetected && (fv.ReadOperations >= 20 || fv.FileAccessRate >= 5 || fv.TotalFileAccesses >= 40) {
		reasons = append(reasons, "suspicious transfer process activity")
	}
	if zAgg >= 6.0 {
		reasons = append(reasons, "strong baseline deviation")
	}
	return reasons
}

func coldStartReasonTags(fv FeatureVector) []string {
	reasons := make([]string, 0, 3)
	if fv.TransferProcessDetected && (fv.ReadOperations >= 20 || fv.FileAccessRate >= 5 || fv.TotalFileAccesses >= 40) {
		reasons = append(reasons, "suspicious transfer process activity during baseline")
	}
	if fv.UID == "0" && (fv.FileAccessRate >= 8 || fv.MaxEventsInShortInterval >= 20 || fv.ReadOperations >= 60) {
		reasons = append(reasons, "privileged user burst activity during baseline")
	}
	if fv.FileAccessRate >= 30 || fv.MaxEventsInShortInterval >= 60 {
		reasons = append(reasons, "extreme burst activity during baseline")
	}
	return reasons
}

func isEntropyOnlyReason(reasons []string) bool {
	return len(reasons) == 1 && reasons[0] == "abnormal entropy of accessed files"
}

func shouldSuppressAlert(reason string, entropyOnly bool, lastAt time.Time, lastReason string, now time.Time) bool {
	if lastAt.IsZero() || lastReason == "" {
		return false
	}
	if reason != lastReason {
		return false
	}

	if entropyOnly {
		return now.Sub(lastAt) < 10*time.Minute
	}
	return now.Sub(lastAt) < 20*time.Second
}
