package writefile

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

var AgentSpec = tools.AgentToolSpec{
	Name:        "write_file",
	Version:     "1",
	Description: "create or overwrite a file",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        "write_file",
	Description: strPtr(sharedtoolmeta.Must("write_file").LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "path to the file to create or overwrite",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "full content to write to the file",
			},
		},
		"required":             []string{"file_path", "content"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
