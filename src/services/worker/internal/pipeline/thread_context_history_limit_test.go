package pipeline

import (
	"testing"
)

func TestCanonicalHistoryFetchLimitUnlimitedWhenNonPositive(t *testing.T) {
	if got := canonicalHistoryFetchLimit(0); got != 0 {
		t.Fatalf("canonicalHistoryFetchLimit(0) = %d, want 0", got)
	}
	if got := canonicalHistoryFetchLimit(-1); got != 0 {
		t.Fatalf("canonicalHistoryFetchLimit(-1) = %d, want 0", got)
	}
}
