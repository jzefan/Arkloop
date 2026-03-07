package documentwrite

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

var AgentSpec = tools.AgentToolSpec{
	Name:        "document_write",
	Version:     "1",
	Description: "write a Markdown document and upload it as a downloadable artifact",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        "document_write",
	Description: strPtr(sharedtoolmeta.Must("document_write").LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename": map[string]any{
				"type":        "string",
				"description": "output filename, must end with .md",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "full Markdown content of the document",
			},
		},
		"required":             []string{"filename", "content"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
