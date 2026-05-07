package exit_plan_mode

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "exit_plan_mode"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "退出 Plan Mode。当前 thread 的 plan 文件保持为最终方案草稿。",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("退出 Plan Mode。调用前应已将当前方案维护在本 thread 的 plan 文件中。"),
	JSONSchema: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
