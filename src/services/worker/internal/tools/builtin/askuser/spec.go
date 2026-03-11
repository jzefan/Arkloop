package askuser

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "ask_user"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "ask the user structured questions with predefined options",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Ask the user structured questions with predefined options. Use when you need the user to make a clear choice between specific options rather than answering open-ended questions."),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"questions": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 3,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string", "description": "unique identifier for the question"},
						"header":   map[string]any{"type": "string", "description": "section header displayed above the question"},
						"question": map[string]any{"type": "string", "description": "the question text"},
						"options": map[string]any{
							"type":     "array",
							"minItems": 2,
							"maxItems": 6,
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"value":       map[string]any{"type": "string"},
									"label":       map[string]any{"type": "string"},
									"description": map[string]any{"type": "string"},
									"recommended": map[string]any{"type": "boolean"},
								},
								"required": []string{"value", "label"},
							},
						},
						"allow_other": map[string]any{"type": "boolean", "description": "whether to allow free-text 'Other' input"},
					},
					"required": []string{"id", "question", "options"},
				},
			},
		},
		"required":             []string{"questions"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
