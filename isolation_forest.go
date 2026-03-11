package main

// isolation_forest.go
//
// Pure-Go implementation of the Isolation Forest algorithm (Liu et al., 2008).
//
// Algorithm summary
// -----------------
//  1. Build NumTrees isolation trees, each on a random sub-sample of the
//     training data of size SampleSize.
//  2. Score a point by the average depth (path length) at which it is isolated
//     across all trees.  Short path → easy to isolate → anomaly.
//  3. Normalise the score to [0, 1] using the expected path length in a random
//     BST: score = 2^(-avgPath / c(SampleSize)).
//  4. During Fit(), score every training point and set the anomaly Threshold
//     at the (1 − Contamination) quantile — values above the threshold are
//     labelled anomalous, mirroring scikit-learn's behaviour.
//
// The trained model is JSON-serialisable so it can be stored in SQLite.

import (
	"encoding/json"
	"math"
	"math/rand"
	"sort"
)

// eulerMascheroni is the Euler–Mascheroni constant γ ≈ 0.5772.
const eulerMascheroni = 0.5772156649015328

// harmonicNumber returns the (n-1)-th harmonic number used in c(n).
func harmonicNumber(n int) float64 {
	if n <= 1 {
		return 0
	}
	return math.Log(float64(n-1)) + eulerMascheroni
}

// avgPathLengthBST returns the expected path length c(n) for a random BST of n nodes.
// This is used to normalise path lengths so that scores stay in [0, 1].
func avgPathLengthBST(n int) float64 {
	switch {
	case n < 2:
		return 0
	case n == 2:
		return 1
	default:
		return 2*harmonicNumber(n) - 2*float64(n-1)/float64(n)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Tree node (flat array, indexed — no pointers, so JSON is trivial)
// ──────────────────────────────────────────────────────────────────────────────

// iTreeNode is a single node in an isolation tree stored in a flat slice.
// Internal nodes carry a split feature + threshold; leaf nodes carry the
// training-sample count for the path-length correction.
type iTreeNode struct {
	IsLeaf     bool    `json:"leaf"`
	Size       int     `json:"size"`  // leaf: #training samples in this partition
	FeatureIdx int     `json:"feat"`  // internal: which feature to test
	SplitValue float64 `json:"split"` // internal: split threshold
	Left       int     `json:"left"`  // index of left child in Nodes slice
	Right      int     `json:"right"` // index of right child in Nodes slice
}

// IsolationTree is one tree stored as a flat node slice.
type IsolationTree struct {
	Nodes []iTreeNode `json:"nodes"`
}

// ──────────────────────────────────────────────────────────────────────────────
// IsolationForest
// ──────────────────────────────────────────────────────────────────────────────

// IsolationForest is the trained ensemble.  It is JSON-serialisable so it can
// be round-tripped through the SQLite baselines table.
type IsolationForest struct {
	Trees         []*IsolationTree `json:"trees"`
	NumTrees      int              `json:"num_trees"`
	SampleSize    int              `json:"sample_size"`
	NumFeatures   int              `json:"num_features"`
	Contamination float64          `json:"contamination"`
	// Threshold is the anomaly score above which a sample is anomalous.
	// Computed during Fit() from the training data.
	Threshold float64 `json:"threshold"`
}

// NewIsolationForest creates an untrained ensemble with the given hyper-parameters.
//
//	numTrees      – number of isolation trees (similar to n_estimators in sklearn, default 100)
//	sampleSize    – sub-sample size per tree (similar to max_samples, default 256)
//	contamination – expected fraction of anomalies in training data (default 0.1)
func NewIsolationForest(numTrees, sampleSize int, contamination float64) *IsolationForest {
	return &IsolationForest{
		NumTrees:      numTrees,
		SampleSize:    sampleSize,
		Contamination: contamination,
	}
}

// Fit trains the forest on data (rows = samples, columns = features).
// After training the model is ready for ScoreAnomaly / IsAnomaly calls.
func (f *IsolationForest) Fit(data [][]float64) {
	if len(data) == 0 || len(data[0]) == 0 {
		return
	}
	f.NumFeatures = len(data[0])
	maxDepth := int(math.Ceil(math.Log2(float64(f.SampleSize))))
	f.Trees = make([]*IsolationTree, f.NumTrees)

	for i := range f.Trees {
		sub := ifSubsample(data, f.SampleSize)
		tree := &IsolationTree{Nodes: make([]iTreeNode, 0, 2*f.SampleSize)}
		ifBuildTree(tree, sub, 0, maxDepth)
		f.Trees[i] = tree
	}

	// Compute the anomaly score for every training sample and use the
	// (1 − contamination) quantile as the decision threshold.
	scores := make([]float64, len(data))
	for i, row := range data {
		scores[i] = f.ScoreAnomaly(row)
	}
	sort.Float64s(scores) // ascending

	idx := int(math.Floor(float64(len(scores)) * (1.0 - f.Contamination)))
	if idx >= len(scores) {
		idx = len(scores) - 1
	}
	f.Threshold = scores[idx]
}

// ScoreAnomaly returns a value in [0, 1]: higher means more anomalous.
// A score > 0.5 generally indicates an anomaly; exact threshold depends on
// Contamination and the training distribution.
func (f *IsolationForest) ScoreAnomaly(sample []float64) float64 {
	if len(f.Trees) == 0 {
		return 0
	}
	total := 0.0
	for _, tree := range f.Trees {
		total += ifPathLength(tree, 0, sample, 0)
	}
	avg := total / float64(len(f.Trees))
	cn := avgPathLengthBST(f.SampleSize)
	if cn == 0 {
		return 0
	}
	return math.Pow(2, -avg/cn)
}

// IsAnomaly returns whether the sample exceeds the trained decision threshold.
// It also returns the raw anomaly score so callers can log it.
func (f *IsolationForest) IsAnomaly(sample []float64) (bool, float64) {
	score := f.ScoreAnomaly(sample)
	return score >= f.Threshold, score
}

// Marshal serialises the trained model to JSON for SQLite storage.
func (f *IsolationForest) Marshal() ([]byte, error) {
	return json.Marshal(f)
}

// UnmarshalIsolationForest restores a model previously serialised with Marshal.
func UnmarshalIsolationForest(data []byte) (*IsolationForest, error) {
	var f IsolationForest
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Tree construction
// ──────────────────────────────────────────────────────────────────────────────

// ifBuildTree recursively partitions data, appending nodes to tree.Nodes.
// It returns the index of the node it created (always the root of the subtree).
func ifBuildTree(tree *IsolationTree, data [][]float64, depth, maxDepth int) int {
	nodeIdx := len(tree.Nodes)
	tree.Nodes = append(tree.Nodes, iTreeNode{}) // reserve slot

	// Stopping conditions: too deep, single sample, or all samples identical
	if len(data) <= 1 || depth >= maxDepth || ifAllIdentical(data) {
		tree.Nodes[nodeIdx] = iTreeNode{IsLeaf: true, Size: len(data)}
		return nodeIdx
	}

	nFeatures := len(data[0])

	// Find a feature with variance to split on (try up to 2× nFeatures attempts)
	featIdx := -1
	var minVal, maxVal float64
	for attempt := 0; attempt < nFeatures*2; attempt++ {
		fi := rand.Intn(nFeatures) //nolint:gosec
		mn, mx := ifFeatureRange(data, fi)
		if mx > mn {
			featIdx = fi
			minVal = mn
			maxVal = mx
			break
		}
	}
	if featIdx == -1 {
		// All features are constant — make a leaf
		tree.Nodes[nodeIdx] = iTreeNode{IsLeaf: true, Size: len(data)}
		return nodeIdx
	}

	splitVal := minVal + rand.Float64()*(maxVal-minVal) //nolint:gosec

	// Partition
	left := make([][]float64, 0, len(data)/2)
	right := make([][]float64, 0, len(data)/2)
	for _, row := range data {
		if row[featIdx] < splitVal {
			left = append(left, row)
		} else {
			right = append(right, row)
		}
	}

	if len(left) == 0 || len(right) == 0 {
		tree.Nodes[nodeIdx] = iTreeNode{IsLeaf: true, Size: len(data)}
		return nodeIdx
	}

	leftIdx := ifBuildTree(tree, left, depth+1, maxDepth)
	rightIdx := ifBuildTree(tree, right, depth+1, maxDepth)

	// Update the reserved slot now that children are known
	tree.Nodes[nodeIdx] = iTreeNode{
		IsLeaf:     false,
		FeatureIdx: featIdx,
		SplitValue: splitVal,
		Left:       leftIdx,
		Right:      rightIdx,
	}
	return nodeIdx
}

// ifPathLength traverses one tree and returns the path length for sample.
// For leaf nodes with size > 1 the expected BST path correction c(size) is added.
func ifPathLength(tree *IsolationTree, nodeIdx int, sample []float64, depth float64) float64 {
	node := tree.Nodes[nodeIdx]
	if node.IsLeaf {
		return depth + avgPathLengthBST(node.Size)
	}
	if sample[node.FeatureIdx] < node.SplitValue {
		return ifPathLength(tree, node.Left, sample, depth+1)
	}
	return ifPathLength(tree, node.Right, sample, depth+1)
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// ifSubsample returns up to n rows from data chosen without replacement.
func ifSubsample(data [][]float64, n int) [][]float64 {
	if n >= len(data) {
		return data
	}
	indices := make([]int, len(data))
	for i := range indices {
		indices[i] = i
	}
	// Partial Fisher-Yates: only shuffle the first n positions
	for i := 0; i < n; i++ {
		j := i + rand.Intn(len(indices)-i) //nolint:gosec
		indices[i], indices[j] = indices[j], indices[i]
	}
	result := make([][]float64, n)
	for i := 0; i < n; i++ {
		result[i] = data[indices[i]]
	}
	return result
}

// ifFeatureRange returns (min, max) for feature fi across data.
func ifFeatureRange(data [][]float64, fi int) (float64, float64) {
	mn, mx := data[0][fi], data[0][fi]
	for _, row := range data[1:] {
		if row[fi] < mn {
			mn = row[fi]
		}
		if row[fi] > mx {
			mx = row[fi]
		}
	}
	return mn, mx
}

// ifAllIdentical returns true if every row in data is element-wise equal.
func ifAllIdentical(data [][]float64) bool {
	if len(data) == 0 {
		return true
	}
	first := data[0]
	for _, row := range data[1:] {
		for j := range row {
			if row[j] != first[j] {
				return false
			}
		}
	}
	return true
}
