package llm

import "testing"

func TestComputeRequestStats_StablePrefixHashIgnoresToolOrderButIncludesSchema(t *testing.T) {
	reqA := Request{
		Messages: []Message{
			{Role: "system", Content: []TextPart{{Text: "stable system"}}},
		},
		Tools: []ToolSpec{
			{Name: "tool_b", JSONSchema: map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "string"}}}},
			{Name: "tool_a", JSONSchema: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}}},
		},
	}
	reqB := Request{
		Messages: []Message{
			{Role: "system", Content: []TextPart{{Text: "stable system"}}},
		},
		Tools: []ToolSpec{
			{Name: "tool_a", JSONSchema: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}}},
			{Name: "tool_b", JSONSchema: map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "string"}}}},
		},
	}
	reqC := Request{
		Messages: []Message{
			{Role: "system", Content: []TextPart{{Text: "stable system"}}},
		},
		Tools: []ToolSpec{
			{Name: "tool_a", JSONSchema: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "number"}}}},
			{Name: "tool_b", JSONSchema: map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "string"}}}},
		},
	}

	hashA := ComputeRequestStats(reqA).StablePrefixHash
	hashB := ComputeRequestStats(reqB).StablePrefixHash
	hashC := ComputeRequestStats(reqC).StablePrefixHash

	if hashA != hashB {
		t.Fatalf("expected tool order to not affect stable prefix hash: %q vs %q", hashA, hashB)
	}
	if hashA == hashC {
		t.Fatalf("expected schema change to affect stable prefix hash: %q", hashA)
	}
}

func TestComputeRequestStats_PromptPlanBucketsAndHashes(t *testing.T) {
	req := Request{
		PromptPlan: &PromptPlan{
			SystemBlocks: []PromptPlanBlock{
				{
					Name:          "persona",
					Target:        PromptTargetSystemPrefix,
					Role:          "system",
					Text:          "stable guardrails",
					Stability:     CacheStabilityStablePrefix,
					CacheEligible: true,
				},
				{
					Name:          "memory_snapshot",
					Target:        PromptTargetSystemPrefix,
					Role:          "system",
					Text:          "session memory",
					Stability:     CacheStabilitySessionPrefix,
					CacheEligible: true,
				},
				{
					Name:          "runtime_context",
					Target:        PromptTargetRuntimeTail,
					Role:          "user",
					Text:          "User Local Now: 2026-04-11 20:00:00",
					Stability:     CacheStabilityVolatileTail,
					CacheEligible: false,
				},
			},
		},
		Tools: []ToolSpec{
			{
				Name:       "web_search",
				JSONSchema: map[string]any{"type": "object"},
				CacheHint:  &CacheHint{Action: CacheHintActionWrite},
			},
		},
	}

	stats := ComputeRequestStats(req)
	if stats.StablePrefixHash == "" {
		t.Fatalf("expected stable prefix hash")
	}
	if stats.SessionPrefixHash == "" {
		t.Fatalf("expected session prefix hash")
	}
	if stats.VolatileTailHash == "" {
		t.Fatalf("expected volatile tail hash")
	}
	if stats.ToolSchemaHash == "" {
		t.Fatalf("expected tool schema hash")
	}
	if stats.StablePrefixBytes == 0 || stats.SessionPrefixBytes == 0 || stats.VolatileTailBytes == 0 {
		t.Fatalf("expected non-zero bucket bytes, got %+v", stats)
	}
	if stats.CacheCandidateBytes == 0 {
		t.Fatalf("expected cache candidate bytes > 0")
	}
}
