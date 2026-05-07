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
	Description: "Use only in heartbeat runs: decide whether to speak or skip this check.",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

// Spec 是 heartbeat_decision 工具的 LLM schema 定义。
var Spec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Use only in heartbeat runs: decide whether to speak or skip this check."),
	JSONSchema: map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"reply"},
		"properties": map[string]any{
			"reply": map[string]any{
				"type":        "boolean",
				"description": "true = speak now, your message will be sent; false = skip this check, nothing happens.",
			},
		},
	},
}

func strPtr(s string) *string { return &s }
