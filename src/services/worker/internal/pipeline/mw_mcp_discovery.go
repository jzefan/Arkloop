package pipeline

import (
	"context"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/mcp"
	"arkloop/services/worker/internal/tools"
)

// NewMCPDiscoveryMiddleware 按 org 从 DB 加载 MCP 工具，合并到 RunContext 的工具集。
func NewMCPDiscoveryMiddleware(
	mcpPool *mcp.Pool,
	baseToolExecutors map[string]tools.Executor,
	baseAllLlmSpecs []llm.ToolSpec,
	baseAllowlistSet map[string]struct{},
	baseRegistry *tools.Registry,
) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		runToolExecutors := CopyToolExecutors(baseToolExecutors)
		runAllLlmSpecs := append([]llm.ToolSpec{}, baseAllLlmSpecs...)
		runAllowlistSet := CopyStringSet(baseAllowlistSet)
		runRegistry := baseRegistry

		if mcpPool != nil {
			orgReg, orgErr := mcp.DiscoverFromDB(ctx, rc.Pool, rc.Run.OrgID, mcpPool)
			if orgErr == nil && len(orgReg.Executors) > 0 {
				runRegistry = ForkRegistry(baseRegistry, orgReg.AgentSpecs)
				for name, exec := range orgReg.Executors {
					runToolExecutors[name] = exec
				}
				runAllLlmSpecs = append(runAllLlmSpecs, orgReg.LlmSpecs...)
				for _, spec := range orgReg.AgentSpecs {
					runAllowlistSet[spec.Name] = struct{}{}
				}
			}
		}

		rc.ToolExecutors = runToolExecutors
		rc.ToolSpecs = runAllLlmSpecs
		rc.AllowlistSet = runAllowlistSet
		rc.ToolRegistry = runRegistry

		return next(ctx, rc)
	}
}
