package pipeline

import (
	"testing"
)

func TestCanonicalHistoryFetchLimitUnlimitedWhenNonPositive(t *testing.T) {
	if got := canonicalHistoryFetchLimit(0); got != canonicalPersistFetchLimit {
		t.Fatalf("canonicalHistoryFetchLimit(0) = %d, want %d", got, canonicalPersistFetchLimit)
	}
	if got := canonicalHistoryFetchLimit(-1); got != canonicalPersistFetchLimit {
		t.Fatalf("canonicalHistoryFetchLimit(-1) = %d, want %d", got, canonicalPersistFetchLimit)
	}
}
