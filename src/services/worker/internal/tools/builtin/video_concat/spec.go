package videoconcat

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "video_concat"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "concatenate multiple mp4 artifacts in order into a single mp4 using ffmpeg",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Concatenate multiple mp4 video artifacts into one final mp4 (in the given order) using ffmpeg's concat demuxer. Inputs are artifact references; output is a new artifact. Use after generating per-shot videos when the user wants a single playable / downloadable film."),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"inputs": map[string]any{
				"type":        "array",
				"description": "ordered list of mp4 artifact references (\"artifact:<key>\" or bare key) to concatenate or wrap with narration audio. Each must belong to the current account.",
				"items":       map[string]any{"type": "string"},
				"minItems":    1,
			},
			"artifact_name": map[string]any{
				"type":        "string",
				"description": "optional output artifact base name (without extension). Defaults to \"final_video\".",
			},
			"reencode": map[string]any{
				"type":        "boolean",
				"description": "optional. When true, re-encode with H.264 instead of stream-copy. Slower but more compatible across mixed codecs/resolutions. Default false (stream copy).",
			},
			"transition": map[string]any{
				"type":        "string",
				"enum":        []string{"none", "crossfade"},
				"description": "optional transition style between clips. Use crossfade to smooth discontinuous generated shots. Default none.",
			},
			"transition_seconds": map[string]any{
				"type":        "number",
				"description": "optional crossfade duration in seconds. Default 0.35, clamped to 0.1-1.0.",
			},
			"audio": map[string]any{
				"type":        "string",
				"description": "optional narration/background audio artifact reference (\"artifact:<key>\" or bare key) to attach to the final mp4.",
			},
			"audio_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"replace", "mix"},
				"description": "optional audio handling. replace uses only the provided audio; mix keeps existing video audio at background_volume and overlays narration. Default mix.",
			},
			"narration_volume": map[string]any{
				"type":        "number",
				"description": "optional volume multiplier for provided audio. Default 1.0.",
			},
			"background_volume": map[string]any{
				"type":        "number",
				"description": "optional volume multiplier for existing video audio when audio_mode=mix. Default 0.25.",
			},
		},
		"required":             []string{"inputs"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
