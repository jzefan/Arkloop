package enter_plan_mode

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "enter_plan_mode"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "进入 Plan Mode：切换到只读探索状态。当任务模糊或跨多个文件、需要先呈现方案再写入时使用。",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("进入 Plan Mode。在 Plan Mode 中，只能使用只读工具（read, grep, glob, web_search, web_fetch, ask_user）。所有写工具（edit, write_file, document_write 等）会被拦截。完成方案后调用 exit_plan_mode 提交 plan 并退出。"),
	JSONSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	},
}

func strPtr(s string) *string { return &s }
