package enter_plan_mode

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "enter_plan_mode"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "进入 Plan Mode：在当前 thread 的 plan 文件中维护方案，再交给用户确认。",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("进入 Plan Mode。当前 thread 的 plan 文件是唯一可维护文件；不要修改普通项目文件。完成方案后调用 exit_plan_mode 退出。"),
	JSONSchema: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
