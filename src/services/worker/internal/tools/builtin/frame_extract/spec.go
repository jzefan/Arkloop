package frameextract

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "frame_extract"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "extract a single frame (PNG) from a video artifact at a specified position (last frame by default), used to chain shot continuity",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Extract a single PNG frame from an mp4 video artifact. By default extracts the last frame, used by orchestrators (e.g. yuhua-stone-director) to chain shot continuity — feed shot N's last frame as shot N+1's first_frame so the two clips visually connect. Supports custom position via 'at_seconds' (number) or 'at' (\"first\" / \"last\"). Returns an image artifact key."),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "mp4 artifact reference (\"artifact:<key>\" or bare key). Must belong to current account.",
			},
			"at": map[string]any{
				"type":        "string",
				"description": "Position keyword: \"first\" or \"last\" (default \"last\"). Ignored when at_seconds is provided.",
			},
			"at_seconds": map[string]any{
				"type":        "number",
				"description": "Exact position in seconds (float). Overrides `at` when provided.",
			},
			"artifact_name": map[string]any{
				"type":        "string",
				"description": "Output artifact base name (no extension). Defaults to \"extracted_frame\".",
			},
		},
		"required":             []string{"input"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
