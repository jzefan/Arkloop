package sandbox

import (
	"strings"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

var (
	CodeExecuteSpec = tools.AgentToolSpec{
		Name:        "code_execute",
		Version:     "1",
		Description: "execute Python code in isolated sandbox and return output",
		RiskLevel:   tools.RiskLevelHigh,
		SideEffects: true,
	}
	ShellExecuteSpec = tools.AgentToolSpec{
		Name:        "shell_execute",
		Version:     "1",
		Description: "execute shell commands in isolated sandbox and return output",
		RiskLevel:   tools.RiskLevelHigh,
		SideEffects: true,
	}
)

var CodeExecuteLlmSpec = llm.ToolSpec{
	Name:        "code_execute",
	Description: stringPtr("execute Python code in an isolated sandbox environment"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code":       map[string]any{"type": "string"},
			"timeout_ms": map[string]any{"type": "integer", "minimum": 1000, "maximum": 300000},
		},
		"required":             []string{"code"},
		"additionalProperties": false,
	},
}

var ShellExecuteLlmSpec = llm.ToolSpec{
	Name:        "shell_execute",
	Description: stringPtr("execute shell commands in an isolated sandbox environment"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string"},
			"timeout_ms": map[string]any{"type": "integer", "minimum": 1000, "maximum": 300000},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	},
}

func AgentSpecs() []tools.AgentToolSpec {
	return []tools.AgentToolSpec{
		CodeExecuteSpec,
		ShellExecuteSpec,
	}
}

func LlmSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		CodeExecuteLlmSpec,
		ShellExecuteLlmSpec,
	}
}

func stringPtr(value string) *string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}
