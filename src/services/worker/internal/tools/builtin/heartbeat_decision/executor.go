package heartbeat_decision

import (
	"context"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"
)

// PipelineBinding 将 RunContext 的写入操作抽象为接口，避免循环导入。
type PipelineBinding interface {
	SetHeartbeatDecisionOutcome(replySilent bool, fragments []string)
	IsHeartbeatRun() bool
}

type executor struct{}

// New 返回 heartbeat_decision executor。
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
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: tools.ErrorClassToolExecutionFailed,
				Message:    "unexpected tool name",
			},
		}
	}

	binding, ok := execCtx.PipelineRC.(PipelineBinding)
	if !ok || binding == nil || !binding.IsHeartbeatRun() {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: tools.ErrorClassToolExecutionFailed,
				Message:    "heartbeat_decision called outside heartbeat run",
			},
		}
	}

	replySilent, _ := args["reply_silent"].(bool)

	var fragments []string
	if raw, ok := args["memory_fragments"].([]any); ok {
		for _, item := range raw {
			if s, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					fragments = append(fragments, trimmed)
				}
			}
		}
	}

	binding.SetHeartbeatDecisionOutcome(replySilent, fragments)

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"ok":              true,
			"reply_silent":    replySilent,
			"fragments_count": len(fragments),
		},
		DurationMs: int(time.Since(started).Milliseconds()),
	}
}
