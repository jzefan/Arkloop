package heartbeat_decision

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "heartbeat_decision"

// AgentSpec 用于 ToolRegistry 注册（ToolBuild → Bind 引用）。
var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "Use only in heartbeat runs: declare intent to reply or stay silent, and persist memory fragments.",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

// Spec 是 heartbeat_decision 工具的 LLM schema 定义。
var Spec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Use only in heartbeat runs: declare intent to reply or stay silent, and persist memory fragments."),
	JSONSchema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"reply"},
		"properties": map[string]any{
			"reply": map[string]any{
				"type":        "boolean",
				"description": "true = send a reply to the user this heartbeat; false = silent, run ends immediately.",
			},
			"memory_fragments": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "short factual notes to persist to long-term memory; only include when there is substantive content worth remembering.",
			},
		},
	},
}

func strPtr(s string) *string { return &s }

// SystemProtocolSnippet 返回注入 system prompt 的心跳协议说明。
func SystemProtocolSnippet() string {
	return "You are in an LLM heartbeat turn. " +
		"SYSTEM CONSTRAINT: calling `" + ToolName + "` with reply=true is the ONLY way any message reaches the user. " +
		"There is no fallback path. " +
		"Call `" + ToolName + "` exactly once in this turn: " +
		"reply=false if you have nothing to say (run ends immediately, write no text); " +
		"reply=true if you want the visible text from this turn to reach the user. " +
		"There is no second step after the tool call in this turn. " +
		"Do not call any other tool in the same turn as `" + ToolName + "`. " +
		"If this turn surfaced facts worth remembering long-term, include them in memory_fragments."
}
