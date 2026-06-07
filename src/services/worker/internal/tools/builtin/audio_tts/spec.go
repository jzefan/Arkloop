package audiotts

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "audio_tts"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "generate speech audio from text and persist it as an artifact",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Generate text-to-speech narration audio from text and save it as an audio artifact. Use this before video_concat when the final mp4 should contain spoken narration."),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "text to speak. Keep it concise enough for the target video duration.",
			},
			"voice": map[string]any{
				"type":        "string",
				"description": "optional voice name supported by the provider. Default alloy.",
			},
			"model_selector": map[string]any{
				"type":        "string",
				"description": "optional routing selector for a TTS model, e.g. gpt-4o-mini-tts or credential^model.",
			},
			"response_format": map[string]any{
				"type":        "string",
				"enum":        []string{"mp3", "wav", "opus", "aac", "flac", "pcm"},
				"description": "optional audio format. Default mp3.",
			},
			"speed": map[string]any{
				"type":        "number",
				"description": "optional speech speed, clamped to 0.25-4.0. Default 1.0.",
			},
			"artifact_name": map[string]any{
				"type":        "string",
				"description": "optional output artifact base name. Defaults to voice_over.",
			},
		},
		"required":             []string{"text"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
