package agent

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
)

// buildPromptCacheDebugPayload 构造单轮 prompt cache 调试事件 payload。
// 严格按计划 §2 schema 输出嵌套结构。仅在 PromptCacheDebugEnabled=true 时调用。
func buildPromptCacheDebugPayload(
	runCtx *pipeline.RunContext,
	turnIndex int,
	cacheEnabled bool,
	stats llm.RequestStats,
	markers llm.PromptCachePlanMarkers,
	breakInfo promptCacheBreakInfo,
	prevTurnUsage map[string]any,
) map[string]any {
	payload := map[string]any{
		"turn_index":    turnIndex,
		"cache_enabled": cacheEnabled,
		"plan":          buildPlanSection(stats),
		"markers":       buildMarkersSection(markers),
		"break":         buildBreakSection(breakInfo),
		"inherited":     buildInheritedSection(runCtx),
	}
	// 首轮无上一轮数据时明确为 null，不省略字段
	if prevTurnUsage == nil {
		payload["usage_prev_turn"] = nil
	} else {
		payload["usage_prev_turn"] = buildUsagePrevTurnSection(prevTurnUsage)
	}
	return payload
}

func buildPlanSection(stats llm.RequestStats) map[string]any {
	return map[string]any{
		"stable_prefix_hash":    stats.StablePrefixHash,
		"stable_prefix_bytes":   stats.StablePrefixBytes,
		"session_prefix_hash":   stats.SessionPrefixHash,
		"session_prefix_bytes":  stats.SessionPrefixBytes,
		"volatile_tail_hash":    stats.VolatileTailHash,
		"volatile_tail_bytes":   stats.VolatileTailBytes,
		"tool_schema_hash":      stats.ToolSchemaHash,
		"cache_candidate_bytes": stats.CacheCandidateBytes,
	}
}

func buildMarkersSection(m llm.PromptCachePlanMarkers) map[string]any {
	ids := m.CacheReferenceToolUseIDs
	if ids == nil {
		ids = []string{}
	}
	return map[string]any{
		"system_blocks_with_cache_control": m.SystemBlocksWithCacheControl,
		"message_cache_control_index":      m.MessageCacheControlIndex,
		"cache_reference_count":            m.CacheReferenceCount,
		"cache_reference_tool_use_ids":     ids,
		"cache_edits_count":                m.CacheEditsCount,
		"stable_marker_applied":            m.StableMarkerApplied,
	}
}

func buildBreakSection(info promptCacheBreakInfo) map[string]any {
	changed := info.ChangedBuckets
	if changed == nil {
		changed = []string{}
	}
	return map[string]any{
		"changed_buckets":   changed,
		"prev_stable_hash":  info.PrevStableHash,
		"curr_stable_hash":  info.CurrStableHash,
		"prev_stable_bytes": info.PrevStableBytes,
		"curr_stable_bytes": info.CurrStableBytes,
	}
}

func buildInheritedSection(runCtx *pipeline.RunContext) map[string]any {
	// LastInheritedReuseResult==nil 表示无 inherited 上下文（非 subagent 路径或 mode=incremental）
	if runCtx == nil || runCtx.LastInheritedReuseResult == nil {
		return map[string]any{
			"has_inherited_snapshot": false,
			"reused":                 false,
			"reuse_failure_reason":   "",
		}
	}
	return map[string]any{
		"has_inherited_snapshot": true,
		"reused":                 runCtx.LastInheritedReuseResult.Reused,
		"reuse_failure_reason":   runCtx.LastInheritedReuseResult.FailureReason,
	}
}

func buildUsagePrevTurnSection(prev map[string]any) map[string]any {
	// 兼容 anthropic/openai 等不同字段命名
	creation := readInt64FromAny(prev, "cache_creation_input_tokens", "cache_creation_tokens")
	read := readInt64FromAny(prev, "cache_read_input_tokens", "cache_read_tokens", "cached_tokens")
	return map[string]any{
		"cache_creation_tokens": creation,
		"cache_read_tokens":     read,
		"was_hit":               read > 0,
	}
}

func readInt64FromAny(payload map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v, ok := payload[key]; ok && v != nil {
			switch n := v.(type) {
			case int64:
				return n
			case int:
				return int64(n)
			case float64:
				return int64(n)
			case int32:
				return int64(n)
			}
		}
	}
	return 0
}
