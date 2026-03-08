package builtin

import (
	"context"
	"sort"
	"strings"
	"time"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const (
	echoErrorArgsInvalid = "tool.args_invalid"
)

var EchoAgentSpec = tools.AgentToolSpec{
	Name:        "echo",
	Version:     "1",
	Description: "echo back input text",
	RiskLevel:   tools.RiskLevelLow,
}

var EchoLlmSpec = llm.ToolSpec{
	Name:        "echo",
	Description: stringPtr("echo back input text"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required":             []string{"text"},
		"additionalProperties": false,
	},
}

type EchoExecutor struct{}

func (EchoExecutor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	_ tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	_ = ctx
	_ = toolName
	started := time.Now()

	unknown := []string{}
	for key := range args {
		if key != "text" {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: echoErrorArgsInvalid,
				Message:    "tool args do not accept extra fields",
				Details:    map[string]any{"unknown_fields": unknown},
			},
			DurationMs: durationMs(started),
		}
	}

	text, ok := args["text"].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return tools.ExecutionResult{
			Error: &tools.ExecutionError{
				ErrorClass: echoErrorArgsInvalid,
				Message:    "parameter text must be a non-empty string",
				Details:    map[string]any{"field": "text"},
			},
			DurationMs: durationMs(started),
		}
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{"text": strings.TrimSpace(text)},
		DurationMs: durationMs(started),
	}
}
