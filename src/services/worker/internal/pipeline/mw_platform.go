package pipeline

import (
	"context"
	"log/slog"
	"strings"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/tools"
	"arkloop/services/worker/internal/tools/builtin/platform"
)

// NewPlatformMiddleware 为 admin 用户注入 platform_manage 工具和 <platform> 状态块。
func NewPlatformMiddleware(platformToolExecutor tools.Executor) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if shouldInjectPlatformTools(ctx, rc, platformToolExecutor) {
			ptSpec := platform.AgentSpec
			rc.ToolExecutors[ptSpec.Name] = platformToolExecutor
			rc.AllowlistSet[ptSpec.Name] = struct{}{}
			rc.ToolSpecs = append(rc.ToolSpecs, platform.LlmSpec)
			rc.ToolRegistry = ForkRegistry(rc.ToolRegistry, []tools.AgentToolSpec{ptSpec})

			upsertPlatformStatusBlock(rc)
		}

		return next(ctx, rc)
	}
}

func shouldInjectPlatformTools(ctx context.Context, rc *RunContext, executor tools.Executor) bool {
	if executor == nil {
		return false
	}

	if rc.PersonaDefinition == nil {
		return false
	}

	// 渠道上下文：复用 RuntimeContextMiddleware 预计算的 SenderIsAdmin
	if rc.ChannelContext != nil {
		return rc.SenderIsAdmin
	}

	// 非渠道上下文（如 Web App）：通过 DB 查询用户角色
	if rc.Run.CreatedByUserID == nil || rc.Pool == nil {
		return false
	}

	repo := data.AccountMembershipsRepository{}
	membership, err := repo.GetByAccountAndUser(ctx, rc.Pool, rc.Run.AccountID, *rc.Run.CreatedByUserID)
	if err != nil {
		slog.WarnContext(ctx, "platform_manage: failed to query membership", "error", err)
		return false
	}
	if membership == nil {
		return false
	}
	return membership.Role == "account_admin" || membership.Role == "platform_admin"
}

func upsertPlatformStatusBlock(rc *RunContext) {
	groups := []string{"web_search", "web_fetch", "memory", "read", "sandbox"}

	var configured []string
	var unconfigured []string

	if rc.ActiveToolProviderByGroup != nil {
		for _, g := range groups {
			if provider, ok := rc.ActiveToolProviderByGroup[g]; ok {
				configured = append(configured, g+": "+provider)
			} else {
				unconfigured = append(unconfigured, g)
			}
		}
	} else {
		unconfigured = groups
	}

	var sb strings.Builder
	sb.WriteString("<platform>\n")

	sb.WriteString("Configured capabilities:\n")
	if len(configured) > 0 {
		for _, c := range configured {
			sb.WriteString("- " + c + "\n")
		}
	} else {
		sb.WriteString("(none)\n")
	}

	sb.WriteString("\nUnconfigured capabilities:\n")
	if len(unconfigured) > 0 {
		for _, u := range unconfigured {
			sb.WriteString("- " + u + "\n")
		}
	} else {
		sb.WriteString("(none)\n")
	}

	sb.WriteString("\nWhen user requests an unconfigured capability, suggest configuring the corresponding tool provider via platform_manage. Use arkloop_help for setup instructions.\n")
	sb.WriteString("</platform>")

	rc.UpsertPromptSegment(PromptSegment{
		Name:          "platform.status",
		Target:        PromptTargetSystemPrefix,
		Role:          "system",
		Text:          sb.String(),
		Stability:     PromptStabilitySessionPrefix,
		CacheEligible: true,
	})
}
