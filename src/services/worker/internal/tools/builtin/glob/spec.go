package glob

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

var AgentSpec = tools.AgentToolSpec{
	Name:        "glob",
	Version:     "1",
	Description: "find files by glob pattern",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        "glob",
	Description: strPtr(sharedtoolmeta.Must("glob").LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "glob pattern to match files (e.g. **/*.go, src/**/*.ts)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "search root directory (default: working directory)",
			},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
