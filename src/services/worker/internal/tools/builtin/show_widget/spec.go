package showwidget

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

var AgentSpec = tools.AgentToolSpec{
	Name:        "show_widget",
	Version:     "1",
	Description: "render an interactive HTML widget inline in the conversation",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        "show_widget",
	Description: strPtr(sharedtoolmeta.Must("show_widget").LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "widget display title",
			},
			"i_have_seen_read_me": map[string]any{
				"type":        "boolean",
				"description": "must be true after calling visualize_read_me or artifact_guidelines in the current run",
			},
			"widget_code": map[string]any{
				"type":        "string",
				"description": "raw HTML fragment. Order: <style>, HTML structure, <script>. MUST be last parameter.",
			},
			"loading_messages": map[string]any{
				"type":        "array",
				"description": "optional short status lines while the widget streams (1-4 items)",
				"items": map[string]any{
					"type": "string",
				},
				"minItems": 1,
				"maxItems": 4,
			},
		},
		"required":             []string{"title", "i_have_seen_read_me", "widget_code"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
