package agent

import (
	"testing"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
)

func TestBuildPromptCacheDebugPayload(t *testing.T) {
	t.Run("first turn usage_prev_turn explicit null", func(t *testing.T) {
		payload := buildPromptCacheDebugPayload(
			nil, 0, true,
			llm.RequestStats{},
			llm.PromptCachePlanMarkers{MessageCacheControlIndex: -1, CacheReferenceToolUseIDs: []string{}},
			promptCacheBreakInfo{},
			nil,
		)
		v, ok := payload["usage_prev_turn"]
		if !ok {
			t.Fatalf("usage_prev_turn missing, expected explicit null key")
		}
		if v != nil {
			t.Fatalf("usage_prev_turn should be nil, got %v", v)
		}
	})

	t.Run("nested schema fields present", func(t *testing.T) {
		stats := llm.RequestStats{
			StablePrefixHash:    "sph",
			StablePrefixBytes:   123,
			SessionPrefixHash:   "sess",
			SessionPrefixBytes:  45,
			VolatileTailHash:    "volt",
			VolatileTailBytes:   67,
			ToolSchemaHash:      "tool",
			CacheCandidateBytes: 999,
		}
		markers := llm.PromptCachePlanMarkers{
			SystemBlocksWithCacheControl: 2,
			MessageCacheControlIndex:     5,
			StableMarkerApplied:          true,
			CacheReferenceCount:          3,
			CacheReferenceToolUseIDs:     []string{"t1", "t2", "t3"},
			CacheEditsCount:              1,
		}
		breakInfo := promptCacheBreakInfo{
			ChangedBuckets:  []string{"stable_prefix"},
			PrevStableHash:  "prev",
			CurrStableHash:  "curr",
			PrevStableBytes: 10,
			CurrStableBytes: 20,
		}
		prev := map[string]any{
			"cache_creation_input_tokens": int64(50),
			"cache_read_input_tokens":     int64(80),
		}
		payload := buildPromptCacheDebugPayload(nil, 2, true, stats, markers, breakInfo, prev)

		if payload["turn_index"] != 2 {
			t.Fatalf("turn_index mismatch")
		}
		if payload["cache_enabled"] != true {
			t.Fatalf("cache_enabled mismatch")
		}
		plan := payload["plan"].(map[string]any)
		if plan["stable_prefix_hash"] != "sph" || plan["cache_candidate_bytes"] != 999 {
			t.Fatalf("plan section incorrect: %+v", plan)
		}
		m := payload["markers"].(map[string]any)
		if m["system_blocks_with_cache_control"] != 2 || m["message_cache_control_index"] != 5 ||
			m["cache_reference_count"] != 3 || m["cache_edits_count"] != 1 ||
			m["stable_marker_applied"] != true {
			t.Fatalf("markers section incorrect: %+v", m)
		}
		ids := m["cache_reference_tool_use_ids"].([]string)
		if len(ids) != 3 {
			t.Fatalf("expected 3 tool_use_ids, got %d", len(ids))
		}
		br := payload["break"].(map[string]any)
		if br["prev_stable_hash"] != "prev" || br["curr_stable_hash"] != "curr" {
			t.Fatalf("break section incorrect: %+v", br)
		}
		changed := br["changed_buckets"].([]string)
		if len(changed) != 1 || changed[0] != "stable_prefix" {
			t.Fatalf("changed_buckets incorrect: %+v", changed)
		}
		if _, ok := br["stable_prefix_preview_prev"]; ok {
			t.Fatalf("break section should not expose stable prefix preview: %+v", br)
		}
		inh := payload["inherited"].(map[string]any)
		if inh["has_inherited_snapshot"] != false {
			t.Fatalf("inherited should be no-snapshot when runCtx nil")
		}
		u := payload["usage_prev_turn"].(map[string]any)
		if u["cache_creation_tokens"] != int64(50) || u["cache_read_tokens"] != int64(80) ||
			u["was_hit"] != true {
			t.Fatalf("usage_prev_turn incorrect: %+v", u)
		}
	})

	t.Run("inherited nil vs failed distinguished", func(t *testing.T) {
		// nil LastInheritedReuseResult = 无 inherited 上下文
		payload := buildPromptCacheDebugPayload(
			&pipeline.RunContext{},
			1, true,
			llm.RequestStats{},
			llm.PromptCachePlanMarkers{MessageCacheControlIndex: -1, CacheReferenceToolUseIDs: []string{}},
			promptCacheBreakInfo{},
			nil,
		)
		inh := payload["inherited"].(map[string]any)
		if inh["has_inherited_snapshot"] != false {
			t.Fatalf("nil should yield has_inherited_snapshot=false")
		}

		// 有 snapshot 但复用失败
		rc := &pipeline.RunContext{
			LastInheritedReuseResult: &pipeline.InheritedReuseResult{
				Reused:        false,
				FailureReason: "tools_mismatch",
			},
		}
		payload2 := buildPromptCacheDebugPayload(
			rc, 1, true,
			llm.RequestStats{},
			llm.PromptCachePlanMarkers{MessageCacheControlIndex: -1, CacheReferenceToolUseIDs: []string{}},
			promptCacheBreakInfo{},
			nil,
		)
		inh2 := payload2["inherited"].(map[string]any)
		if inh2["has_inherited_snapshot"] != true ||
			inh2["reused"] != false ||
			inh2["reuse_failure_reason"] != "tools_mismatch" {
			t.Fatalf("inherited failed case incorrect: %+v", inh2)
		}
	})

	t.Run("inherited reused success", func(t *testing.T) {
		rc := &pipeline.RunContext{
			LastInheritedReuseResult: &pipeline.InheritedReuseResult{Reused: true},
		}
		payload := buildPromptCacheDebugPayload(
			rc, 1, true,
			llm.RequestStats{},
			llm.PromptCachePlanMarkers{MessageCacheControlIndex: -1, CacheReferenceToolUseIDs: []string{}},
			promptCacheBreakInfo{},
			nil,
		)
		inh := payload["inherited"].(map[string]any)
		if inh["has_inherited_snapshot"] != true || inh["reused"] != true {
			t.Fatalf("expected reused=true with snapshot: %+v", inh)
		}
		if inh["reuse_failure_reason"] != "" {
			t.Fatalf("expected empty failure reason on success: %+v", inh)
		}
	})

	t.Run("inherited tools_mismatch failure", func(t *testing.T) {
		rc := &pipeline.RunContext{
			LastInheritedReuseResult: &pipeline.InheritedReuseResult{
				Reused:        false,
				FailureReason: "tools_mismatch",
			},
		}
		payload := buildPromptCacheDebugPayload(
			rc, 1, true,
			llm.RequestStats{},
			llm.PromptCachePlanMarkers{MessageCacheControlIndex: -1, CacheReferenceToolUseIDs: []string{}},
			promptCacheBreakInfo{},
			nil,
		)
		inh := payload["inherited"].(map[string]any)
		if inh["has_inherited_snapshot"] != true ||
			inh["reused"] != false ||
			inh["reuse_failure_reason"] != "tools_mismatch" {
			t.Fatalf("tools_mismatch case incorrect: %+v", inh)
		}
	})

	t.Run("openai key variants recognized", func(t *testing.T) {
		prev := map[string]any{
			"cached_tokens": int64(42),
		}
		payload := buildPromptCacheDebugPayload(
			nil, 1, true,
			llm.RequestStats{},
			llm.PromptCachePlanMarkers{MessageCacheControlIndex: -1, CacheReferenceToolUseIDs: []string{}},
			promptCacheBreakInfo{},
			prev,
		)
		u := payload["usage_prev_turn"].(map[string]any)
		if u["cache_read_tokens"] != int64(42) || u["was_hit"] != true {
			t.Fatalf("openai cached_tokens not read: %+v", u)
		}
	})
}
