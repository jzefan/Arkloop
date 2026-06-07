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
	Description: "generate a short video clip (mp4) from a text prompt and optional first-frame image",
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
				"description": "the full video generation prompt (text describing motion, camera, mood)",
			},
			"first_frame": map[string]any{
				"type":        "string",
				"description": "optional artifact reference (artifact:<key>) used as the first frame for image-to-video. When omitted the model runs in text-to-video mode.",
			},
			"duration_seconds": map[string]any{
				"type":        "integer",
				"description": "optional clip duration in seconds (3-10, default 5)",
			},
			"resolution": map[string]any{
				"type":        "string",
				"description": "optional output resolution, e.g. \"480p\" (default) or \"720p\"",
			},
			"model_selector": map[string]any{
				"type":        "string",
				"description": "optional routing selector to pin a specific video model for this call. Format: bare model name like \"doubao-seedance-1-0-pro-250528\" or \"credName^modelName\" for exact pinning.",
			},
			"artifact_name": map[string]any{
				"type":        "string",
				"description": "optional artifact base name (without extension) to disambiguate multiple video_generate calls within the same run. Defaults to \"generated-video\".",
			},
		},
		"required":             []string{"prompt"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
