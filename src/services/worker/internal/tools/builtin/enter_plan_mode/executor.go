package enter_plan_mode

import (
	"context"
	"fmt"
	"time"

	"arkloop/services/worker/internal/tools"
)

type PipelineBinding interface {
	SetIsPlanMode(active bool)
	SetPlanFilePath(path string)
}

type executor struct{}

func New() tools.Executor {
	return executor{}
}

func (executor) Execute(
	ctx context.Context,
	toolName string,
	_ map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if toolName != ToolName {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: tools.ErrorClassToolExecutionFailed,
				Message:    "unexpected tool name",
			},
			DurationMs: int(time.Since(started).Milliseconds()),
		}
	}

	binding, ok := execCtx.PipelineRC.(PipelineBinding)
	if !ok || binding == nil {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: tools.ErrorClassToolExecutionFailed,
				Message:    "enter_plan_mode: pipeline binding unavailable",
			},
			DurationMs: int(time.Since(started).Milliseconds()),
		}
	}

	planPath := "plans/current.md"
	if execCtx.ThreadID != nil {
		planPath = fmt.Sprintf("plans/%s.md", execCtx.ThreadID.String())
	}

	binding.SetIsPlanMode(true)
	binding.SetPlanFilePath(planPath)

	instructions := "你已进入 Plan Mode。\n\n" +
		"在此模式下，你只能使用只读工具：read, grep, glob, web_search, web_fetch, ask_user。\n" +
		"以下写工具被禁用：edit, write_file, document_write, exec_command（写命令）, python_execute（写操作）。\n\n" +
		"目标：探索代码、理解架构、设计实现方案。\n\n" +
		"完成方案后，先用普通文本向用户呈现 plan 概要并等待用户确认，再调用 exit_plan_mode 提交完整 plan 并退出。Plan 将被写入：" + planPath

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"status":         "plan_mode_entered",
			"plan_file_path": planPath,
			"instructions":   instructions,
		},
		DurationMs: int(time.Since(started).Milliseconds()),
	}
}
