package exit_plan_mode

import (
	"context"
	"fmt"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"
	"arkloop/services/worker/internal/tools/builtin/fileops"
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
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if toolName != ToolName {
		return errResult("unexpected tool name", started)
	}

	plan, _ := args["plan"].(string)
	plan = strings.TrimSpace(plan)
	if plan == "" {
		return errResult("plan is required", started)
	}

	binding, ok := execCtx.PipelineRC.(PipelineBinding)
	if !ok || binding == nil {
		return errResult("exit_plan_mode: pipeline binding unavailable", started)
	}

	if !binding.IsPlanModeActive() {
		return errResult("not in plan mode", started)
	}

	planPath := binding.PlanFilePathValue()
	if planPath == "" {
		if execCtx.ThreadID != nil {
			planPath = fmt.Sprintf("plans/%s.md", execCtx.ThreadID.String())
		} else {
			planPath = "plans/current.md"
		}
	}

	backend := fileops.ResolveBackend(
		execCtx.RuntimeSnapshot,
		execCtx.WorkDir,
		execCtx.RunID.String(),
		resolveAccountID(execCtx),
		execCtx.ProfileRef,
		execCtx.WorkspaceRef,
	)
	if err := backend.WriteFile(ctx, planPath, []byte(plan)); err != nil {
		return errResult(fmt.Sprintf("write plan failed: %s", err.Error()), started)
	}

	binding.SetIsPlanMode(false)

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"status":         "plan_mode_exited",
			"plan_file_path": planPath,
			"bytes_written":  len(plan),
		},
		DurationMs: int(time.Since(started).Milliseconds()),
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

func resolveAccountID(execCtx tools.ExecutionContext) string {
	if execCtx.AccountID == nil {
		return ""
	}
	return execCtx.AccountID.String()
}
