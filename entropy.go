package main

import (
	"math"
	"strings"
)

// ShannonEntropy computes entropy from a frequency map.
func ShannonEntropy(counts map[string]int) float64 {
	if len(counts) == 0 {
		return 0
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0
	}

	entropy := 0.0
	totalF := float64(total)
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / totalF
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// FilenamePattern normalizes a filename shape to identify structured collection.
// Letters become 'a', digits become 'd', separators are preserved.
func FilenamePattern(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteByte('a')
		case r >= 'A' && r <= 'Z':
			b.WriteByte('a')
		case r >= '0' && r <= '9':
			b.WriteByte('d')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
