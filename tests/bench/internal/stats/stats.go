package stats

import (
	"errors"
	"math"
	"sort"
)

var ErrEmptySample = errors.New("empty sample")

type Summary struct {
	Count int `json:"count"`

	MinMs float64 `json:"min_ms"`
	P50Ms float64 `json:"p50_ms"`
	P90Ms float64 `json:"p90_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
	MaxMs float64 `json:"max_ms"`
	AvgMs float64 `json:"avg_ms"`
}

func MergeFloat64(dst []float64, src ...[]float64) []float64 {
	for _, s := range src {
		dst = append(dst, s...)
	}
	return dst
}

func SummarizeMs(values []float64) (Summary, error) {
	if len(values) == 0 {
		return Summary{}, ErrEmptySample
	}

	cp := make([]float64, len(values))
	copy(cp, values)
	sort.Float64s(cp)

	var sum float64
	for _, v := range cp {
		sum += v
	}

	return Summary{
		Count: len(cp),
		MinMs: cp[0],
		P50Ms: percentileSorted(cp, 0.50),
		P90Ms: percentileSorted(cp, 0.90),
		P95Ms: percentileSorted(cp, 0.95),
		P99Ms: percentileSorted(cp, 0.99),
		MaxMs: cp[len(cp)-1],
		AvgMs: sum / float64(len(cp)),
	}, nil
}

func percentileSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	// nearest-rank
	n := float64(len(sorted))
	rank := int(math.Ceil(p*n)) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}
