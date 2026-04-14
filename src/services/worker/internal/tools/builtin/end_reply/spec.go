package end_reply

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "end_reply"

// AgentSpec 用于 ToolRegistry 注册。
var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "End the current reply without producing further text output. Use when you want to stay silent (e.g., after reacting with an emoji) or to avoid repeating content after a tool call. Do NOT call at the end of normal text replies — they end naturally.",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

// Spec 是 end_reply 工具的 LLM schema 定义。
var Spec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("End the current reply without producing further text output. Use when you want to stay silent (e.g., after reacting with an emoji) or to avoid repeating content after a tool call. Do NOT call at the end of normal text replies — they end naturally."),
	JSONSchema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
	},
}

func strPtr(s string) *string { return &s }
