package exit_plan_mode

import (
	"context"
	"fmt"
	"time"

	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/tools"
)

type PipelineBinding interface {
	SetIsPlanMode(active bool)
	PlanFilePathValue() string
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
		return errResult("unexpected tool name", started)
	}

	binding, ok := execCtx.PipelineRC.(PipelineBinding)
	if !ok || binding == nil {
		return errResult("exit_plan_mode: pipeline binding unavailable", started)
	}

	if !binding.IsPlanModeActive() {
		return errResult("not in plan mode", started)
	}
	if execCtx.ThreadID == nil {
		return errResult("exit_plan_mode: thread_id is required", started)
	}

	planPath := binding.PlanFilePathValue()
	if planPath == "" {
		planPath = fmt.Sprintf("plans/%s.md", execCtx.ThreadID.String())
	}

	binding.SetIsPlanMode(false)
	event := execCtx.Emitter.Emit("thread.plan_mode.updated", map[string]any{
		"thread_id":      execCtx.ThreadID.String(),
		"plan_mode":      false,
		"plan_file_path": planPath,
		"source":         "tool",
	}, nil, nil)

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"status":         "plan_mode_exited",
			"plan_file_path": planPath,
		},
		DurationMs: int(time.Since(started).Milliseconds()),
		Events:     []events.RunEvent{event},
	}
}

func errResult(message string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: tools.ErrorClassToolExecutionFailed,
			Message:    message,
		},
		DurationMs: int(time.Since(started).Milliseconds()),
	}
}
