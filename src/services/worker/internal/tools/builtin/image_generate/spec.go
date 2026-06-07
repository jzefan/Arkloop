package imagegenerate

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "image_generate"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "generate an image and save it as an artifact",
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
				"description": "the full image generation prompt",
			},
			"input_images": map[string]any{
				"type":        "array",
				"description": "optional source images as artifact references (artifact:<key>) or current-message image attachments (attachment:<key>)",
				"items": map[string]any{
					"type": "string",
				},
			},
			"size": map[string]any{
				"type":        "string",
				"description": "optional output size, for example 1024x1024",
			},
			"quality": map[string]any{
				"type":        "string",
				"description": "optional output quality, for example low, medium, high, or auto",
			},
			"background": map[string]any{
				"type":        "string",
				"description": "optional background mode, for example transparent or opaque",
			},
			"output_format": map[string]any{
				"type":        "string",
				"description": "optional output image format, for example png, jpeg, or webp",
			},
			"model_selector": map[string]any{
				"type":        "string",
				"description": "optional routing selector to pin a specific image model for this call (overrides account-level image_generative.model). Format: bare model name like \"openai/gpt-5-image-mini\" matches by route.model; or \"credName^modelName\" for exact credential+model pinning.",
			},
			"artifact_name": map[string]any{
				"type":        "string",
				"description": "optional artifact base name (without extension) to disambiguate multiple image_generate calls within the same run. When omitted, defaults to \"generated-image\" — which means repeated calls in the same run will overwrite each other. Use distinct names like \"character_reference_sheet\" or \"shot_01\" when calling this tool more than once per run.",
			},
		},
		"required":             []string{"prompt"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
