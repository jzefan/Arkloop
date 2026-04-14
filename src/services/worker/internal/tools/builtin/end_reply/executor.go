package end_reply

import (
	"context"
	"time"

	"arkloop/services/worker/internal/tools"
)

// PipelineBinding 将 RunContext 的写入操作抽象为接口，避免循环导入。
type PipelineBinding interface {
	SetEndReplyRequested(requested bool)
}

type executor struct{}

// New 返回 end_reply executor。
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
		}
	}

	binding, ok := execCtx.PipelineRC.(PipelineBinding)
	if !ok || binding == nil {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: tools.ErrorClassToolExecutionFailed,
				Message:    "end_reply: pipeline binding unavailable",
			},
		}
	}

	binding.SetEndReplyRequested(true)

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"status": "reply_ended",
		},
		DurationMs: int(time.Since(started).Milliseconds()),
	}
}
