package pipeline

import (
	"log/slog"
	"sort"
	"strings"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

func CopyToolExecutors(src map[string]tools.Executor) map[string]tools.Executor {
	out := make(map[string]tools.Executor, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func CopyStringSet(src map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(src))
	for k := range src {
		out[k] = struct{}{}
	}
	return out
}

// ForkRegistry 创建一个包含 base 所有 spec + 额外 spec 的新 Registry。
func ForkRegistry(base *tools.Registry, extras []tools.AgentToolSpec) *tools.Registry {
	r := tools.NewRegistry()
	for _, name := range base.ListNames() {
		spec, ok := base.Get(name)
		if ok {
			_ = r.Register(spec)
		}
	}
	for _, spec := range extras {
		if err := r.Register(spec); err != nil {
			slog.Warn("mcp tool name conflict, skipped", "name", spec.Name)
		}
	}
	return r
}

func BuildDispatchExecutor(
	registry *tools.Registry,
	executors map[string]tools.Executor,
	allowlistSet map[string]struct{},
) (*tools.DispatchingExecutor, error) {
	allowlistNames := make([]string, 0, len(allowlistSet))
	for name := range allowlistSet {
		allowlistNames = append(allowlistNames, name)
	}
	sort.Strings(allowlistNames)

	allowlist := tools.AllowlistFromNames(allowlistNames)
	policy := tools.NewPolicyEnforcer(registry, allowlist)
	dispatch := tools.NewDispatchingExecutor(registry, policy)
	for toolName, bound := range executors {
		if err := dispatch.Bind(toolName, bound); err != nil {
			return nil, err
		}
	}
	return dispatch, nil
}

func FilterToolSpecs(specs []llm.ToolSpec, allowlistSet map[string]struct{}) []llm.ToolSpec {
	if len(allowlistSet) == 0 {
		return nil
	}
	out := make([]llm.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		if _, ok := allowlistSet[spec.Name]; !ok {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func StringPtr(value string) *string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}
