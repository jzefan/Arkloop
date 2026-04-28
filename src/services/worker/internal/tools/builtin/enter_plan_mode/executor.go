package enter_plan_mode

import (
	"context"
	"fmt"
	"time"

	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/tools"
)

type PipelineBinding interface {
	SetIsPlanMode(active bool)
	SetPlanFilePath(path string)
	IsPlanModeActive() bool
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

	if execCtx.ThreadID == nil {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: tools.ErrorClassToolExecutionFailed,
				Message:    "enter_plan_mode: thread_id is required",
			},
			DurationMs: int(time.Since(started).Milliseconds()),
		}
	}
	planPath := fmt.Sprintf("plans/%s.md", execCtx.ThreadID.String())
	if binding.IsPlanModeActive() {
		binding.SetPlanFilePath(planPath)
		return tools.ExecutionResult{
			ResultJSON: map[string]any{
				"status":         "already_in_plan",
				"plan_file_path": planPath,
			},
			DurationMs: int(time.Since(started).Milliseconds()),
		}
	}

	binding.SetIsPlanMode(true)
	binding.SetPlanFilePath(planPath)

	instructions := "你已进入 Plan Mode。\n\n" +
		"当前 Plan Mode 绑定到这个 thread。请把方案持续维护在 Plan 文件：" + planPath + "。\n" +
		"可以读取代码、搜索、提问并修订 Plan 文件；不要修改普通项目文件或执行会改变项目状态的命令。\n" +
		"当方案已经准备好交给用户确认时，调用 exit_plan_mode 退出。"

	event := execCtx.Emitter.Emit("thread.collaboration_mode.updated", map[string]any{
		"thread_id":                   execCtx.ThreadID.String(),
		"run_id":                      execCtx.RunID.String(),
		"previous_collaboration_mode": "default",
		"collaboration_mode":          "plan",
		"plan_file_path":              planPath,
		"source":                      "model",
	}, nil, nil)

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"status":         "plan_mode_entered",
			"plan_file_path": planPath,
			"instructions":   instructions,
		},
		DurationMs: int(time.Since(started).Milliseconds()),
		Events:     []events.RunEvent{event},
	}
}
