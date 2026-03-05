package entitlement

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestResolverNilPoolFallbackToDefault(t *testing.T) {
	r := NewResolver(nil, nil)
	val, err := r.Resolve(context.Background(), uuid.New(), "quota.runs_per_month")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "999999" {
		t.Fatalf("expected registry default 999999, got %q", val)
	}
}

func TestResolverNilPoolCountsReturnZero(t *testing.T) {
	r := NewResolver(nil, nil)
	orgID := uuid.New()
	now := time.Now().UTC()

	runs, err := r.CountMonthlyRuns(context.Background(), orgID, now.Year(), int(now.Month()))
	if err != nil {
		t.Fatalf("CountMonthlyRuns error: %v", err)
	}
	if runs != 0 {
		t.Fatalf("CountMonthlyRuns = %d, want 0", runs)
	}

	tokens, err := r.SumMonthlyTokens(context.Background(), orgID, now.Year(), int(now.Month()))
	if err != nil {
		t.Fatalf("SumMonthlyTokens error: %v", err)
	}
	if tokens != 0 {
		t.Fatalf("SumMonthlyTokens = %d, want 0", tokens)
	}

	balance, err := r.GetCreditBalance(context.Background(), orgID)
	if err != nil {
		t.Fatalf("GetCreditBalance error: %v", err)
	}
	if balance != 0 {
		t.Fatalf("GetCreditBalance = %d, want 0", balance)
	}
}

func TestResolverResolveIntParsesDefault(t *testing.T) {
	r := NewResolver(nil, nil)
	orgID := uuid.New()

	runs, err := r.ResolveInt(context.Background(), orgID, "quota.runs_per_month")
	if err != nil {
		t.Fatalf("ResolveInt runs: %v", err)
	}
	if runs != 999999 {
		t.Fatalf("ResolveInt runs = %d, want 999999", runs)
	}

	tokens, err := r.ResolveInt(context.Background(), orgID, "quota.tokens_per_month")
	if err != nil {
		t.Fatalf("ResolveInt tokens: %v", err)
	}
	if tokens != 1000000 {
		t.Fatalf("ResolveInt tokens = %d, want 1000000", tokens)
	}
}

func TestMonthRange(t *testing.T) {
	tests := []struct {
		year      int
		month     int
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			year:      2025,
			month:     1,
			wantStart: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			year:      2024,
			month:     12,
			wantStart: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			year:      2024,
			month:     2,
			wantStart: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		start, end := monthRange(tt.year, tt.month)
		if !start.Equal(tt.wantStart) {
			t.Fatalf("monthRange(%d, %d) start = %v, want %v", tt.year, tt.month, start, tt.wantStart)
		}
		if !end.Equal(tt.wantEnd) {
			t.Fatalf("monthRange(%d, %d) end = %v, want %v", tt.year, tt.month, end, tt.wantEnd)
		}
	}
}
