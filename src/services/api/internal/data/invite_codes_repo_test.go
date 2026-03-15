package data

import "testing"

func TestNormalizeInviteCodeMaxUses(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  int
	}{
		{name: "negative becomes unlimited", input: -1, want: 0},
		{name: "zero stays unlimited", input: 0, want: 0},
		{name: "positive is preserved", input: 7, want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeInviteCodeMaxUses(tt.input); got != tt.want {
				t.Fatalf("NormalizeInviteCodeMaxUses(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestInviteCodeHasRemainingUses(t *testing.T) {
	tests := []struct {
		name     string
		useCount int
		maxUses  int
		want     bool
	}{
		{name: "zero max uses is unlimited", useCount: 99, maxUses: 0, want: true},
		{name: "negative max uses is unlimited", useCount: 99, maxUses: -1, want: true},
		{name: "positive max uses still enforces limit", useCount: 1, maxUses: 2, want: true},
		{name: "positive max uses blocks at limit", useCount: 2, maxUses: 2, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InviteCodeHasRemainingUses(tt.useCount, tt.maxUses); got != tt.want {
				t.Fatalf("InviteCodeHasRemainingUses(%d, %d) = %v, want %v", tt.useCount, tt.maxUses, got, tt.want)
			}
		})
	}
}
