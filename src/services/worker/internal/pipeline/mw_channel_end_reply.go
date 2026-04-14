package pipeline

import (
	"context"
	"strings"

	"arkloop/services/worker/internal/tools"
	"arkloop/services/worker/internal/tools/builtin/end_reply"
)

// NewChannelEndReplyMiddleware 在 channel run 上注入 end_reply 工具。
// 非 channel 场景（rc.ChannelContext == nil）直接透传。
func NewChannelEndReplyMiddleware() RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc == nil || rc.ChannelContext == nil {
			return next(ctx, rc)
		}

		deny := make(map[string]struct{})
		for _, n := range rc.ToolDenylist {
			if c := strings.TrimSpace(n); c != "" {
				deny[c] = struct{}{}
			}
		}

		if _, blocked := deny[end_reply.ToolName]; blocked {
			return next(ctx, rc)
		}

		exec := end_reply.New()
		rc.ToolExecutors[end_reply.ToolName] = exec
		rc.AllowlistSet[end_reply.ToolName] = struct{}{}
		rc.ToolSpecs = append(rc.ToolSpecs, end_reply.Spec)
		rc.ToolRegistry = ForkRegistry(rc.ToolRegistry, []tools.AgentToolSpec{end_reply.AgentSpec})

		return next(ctx, rc)
	}
}
