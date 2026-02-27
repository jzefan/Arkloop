package builtin

import (
	"arkloop/services/worker/internal/tools/builtin/browser"
	webfetch "arkloop/services/worker/internal/tools/builtin/web_fetch"
	websearch "arkloop/services/worker/internal/tools/builtin/web_search"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

func AgentSpecs() []tools.AgentToolSpec {
	specs := []tools.AgentToolSpec{
		EchoAgentSpec,
		NoopAgentSpec,
		websearch.AgentSpec,
		webfetch.AgentSpec,
	}
	if browser.BaseURLFromEnv() != "" {
		specs = append(specs, browser.AgentSpecs()...)
	}
	return specs
}

func LlmSpecs() []llm.ToolSpec {
	specs := []llm.ToolSpec{
		EchoLlmSpec,
		NoopLlmSpec,
		websearch.LlmSpec,
		webfetch.LlmSpec,
	}
	if browser.BaseURLFromEnv() != "" {
		specs = append(specs, browser.LlmSpecs()...)
	}
	return specs
}

// Executors 返回所有内置工具的 Executor 实例。
// pool 可选；非 nil 时工具配置优先从 platform_settings 读取，回退到 ENV。
func Executors(pool *pgxpool.Pool) map[string]tools.Executor {
	m := map[string]tools.Executor{
		EchoAgentSpec.Name:       EchoExecutor{},
		NoopAgentSpec.Name:       NoopExecutor{},
		websearch.AgentSpec.Name: websearch.NewToolExecutor(pool),
		webfetch.AgentSpec.Name:  webfetch.NewToolExecutor(pool),
	}
	if baseURL := browser.BaseURLFromEnv(); baseURL != "" {
		exec := browser.NewToolExecutor(baseURL)
		for _, spec := range browser.AgentSpecs() {
			m[spec.Name] = exec
		}
	}
	return m
}

