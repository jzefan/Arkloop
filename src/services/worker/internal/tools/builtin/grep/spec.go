package grep

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

var AgentSpec = tools.AgentToolSpec{
	Name:        "grep",
	Version:     "1",
	Description: "search file contents by regex pattern",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        "grep",
	Description: strPtr(sharedtoolmeta.Must("grep").LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "regex pattern to search for in file contents",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "search root directory (default: working directory)",
			},
			"include": map[string]any{
				"type":        "string",
				"description": "file glob to restrict search (e.g. *.go, *.ts)",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
