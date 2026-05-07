package videogenerate

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "video_generate"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "generate a video and save it as an artifact",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr(sharedtoolmeta.Must(ToolName).LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "the full video generation prompt",
			},
			"aspect_ratio": map[string]any{
				"type":        "string",
				"description": "optional output aspect ratio, for example 16:9 or 9:16",
			},
			"resolution": map[string]any{
				"type":        "string",
				"description": "optional output resolution, for example 720p or 1080p",
			},
			"duration_seconds": map[string]any{
				"type":        "number",
				"description": "optional duration in seconds",
			},
			"fps": map[string]any{
				"type":        "number",
				"description": "optional frames per second",
			},
			"negative_prompt": map[string]any{
				"type":        "string",
				"description": "optional negative prompt",
			},
			"generate_audio": map[string]any{
				"type":        "boolean",
				"description": "optional flag to request generated audio when the model supports it",
			},
		},
		"required":             []string{"prompt"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
