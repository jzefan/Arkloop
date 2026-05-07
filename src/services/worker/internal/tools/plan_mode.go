package tools

import "time"

// PlanModeChecker is implemented by ExecutionContext.PipelineRC (the pipeline RunContext)
// to expose plan-mode state to write tools without importing the pipeline package.
type PlanModeChecker interface {
	IsPlanModeActive() bool
}

type PlanModeWritePathChecker interface {
	PlanModeChecker
	PlanModeWritePathAllowed(path string) bool
}

// PlanModeBlocked returns a populated ExecutionResult when plan mode is active,
// otherwise it returns the zero-value result and false. Write tools should call this
// at the top of Execute and short-circuit when blocked is true.
func PlanModeBlocked(rc any, started time.Time) (ExecutionResult, bool) {
	checker, ok := rc.(PlanModeChecker)
	if !ok || checker == nil || !checker.IsPlanModeActive() {
		return ExecutionResult{}, false
	}
	return ExecutionResult{
		Error: &ExecutionError{
			ErrorClass: ErrorClassToolExecutionFailed,
			Message:    "tool not available in plan mode (call exit_plan_mode to leave)",
		},
		DurationMs: int(time.Since(started).Milliseconds()),
	}, true
}

func PlanModeWriteBlocked(rc any, started time.Time, filePath string) (ExecutionResult, bool) {
	checker, ok := rc.(PlanModeChecker)
	if !ok || checker == nil || !checker.IsPlanModeActive() {
		return ExecutionResult{}, false
	}
	if pathChecker, ok := rc.(PlanModeWritePathChecker); ok && pathChecker.PlanModeWritePathAllowed(filePath) {
		return ExecutionResult{}, false
	}
	return ExecutionResult{
		Error: &ExecutionError{
			ErrorClass: ErrorClassToolExecutionFailed,
			Message:    "tool not available in plan mode outside the current plan file",
		},
		DurationMs: int(time.Since(started).Milliseconds()),
	}, true
}
