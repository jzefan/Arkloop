package exit_plan_mode

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "exit_plan_mode"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "退出 Plan Mode 并将最终 plan 写入 workspace。仅在已向用户呈现 plan 并获得确认后调用。",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("退出 Plan Mode。将 plan 内容写入 workspace 的 plan 文件并恢复写工具。plan 参数为最终 plan 的 markdown 内容。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan": map[string]any{
				"type":        "string",
				"description": "最终 plan 内容（markdown），将被写入 workspace 的 plan 文件。",
			},
		},
		"required":             []string{"plan"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
