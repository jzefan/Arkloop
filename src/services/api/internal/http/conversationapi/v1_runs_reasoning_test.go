package conversationapi

import "testing"

func TestNormalizeRunReasoningMode(t *testing.T) {
	tests := map[string]string{
		"auto":       "auto",
		"enabled":    "enabled",
		"disabled":   "disabled",
		"none":       "none",
		"minimal":    "minimal",
		"low":        "low",
		"medium":     "medium",
		"high":       "high",
		"xhigh":      "xhigh",
		"extra-high": "xhigh",
	}

	for input, want := range tests {
		if got := normalizeRunReasoningMode(input); got != want {
			t.Fatalf("normalizeRunReasoningMode(%q) = %q, want %q", input, got, want)
		}
	}

	if got := normalizeRunReasoningMode("unknown"); got != "" {
		t.Fatalf("normalizeRunReasoningMode(unknown) = %q, want empty", got)
	}
}
