package stats

import (
	"errors"
	"testing"
)

func TestSummarizeMsPercentilesNearestRank(t *testing.T) {
	values := []float64{5, 1, 2, 3, 4}
	s, err := SummarizeMs(values)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}

	if s.Count != 5 {
		t.Fatalf("count=%d", s.Count)
	}
	if s.MinMs != 1 || s.MaxMs != 5 {
		t.Fatalf("min/max=%v/%v", s.MinMs, s.MaxMs)
	}
	if s.P50Ms != 3 {
		t.Fatalf("p50=%v", s.P50Ms)
	}
	if s.P90Ms != 5 {
		t.Fatalf("p90=%v", s.P90Ms)
	}
	if s.P95Ms != 5 {
		t.Fatalf("p95=%v", s.P95Ms)
	}
	if s.P99Ms != 5 {
		t.Fatalf("p99=%v", s.P99Ms)
	}
}

func TestSummarizeMsEmpty(t *testing.T) {
	_, err := SummarizeMs(nil)
	if !errors.Is(err, ErrEmptySample) {
		t.Fatalf("err=%v", err)
	}
}

func TestMergeFloat64(t *testing.T) {
	out := MergeFloat64([]float64{1}, []float64{2, 3}, nil, []float64{})
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0] != 1 || out[1] != 2 || out[2] != 3 {
		t.Fatalf("out=%v", out)
	}
}
