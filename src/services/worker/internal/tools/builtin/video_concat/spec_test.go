package videoconcat

import (
	"strings"
	"testing"
)

func TestVideoConcatSpecExposesCrossfadeTransition(t *testing.T) {
	props, ok := LlmSpec.JSONSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing from schema: %#v", LlmSpec.JSONSchema)
	}
	inputs, ok := props["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("inputs missing from schema: %#v", props["inputs"])
	}
	if inputs["minItems"] != 1 {
		t.Fatalf("expected inputs minItems=1 so a single clip can receive narration audio, got %#v", inputs["minItems"])
	}
	if _, ok := props["transition"]; !ok {
		t.Fatal("expected transition property")
	}
	if _, ok := props["transition_seconds"]; !ok {
		t.Fatal("expected transition_seconds property")
	}
}

func TestVideoConcatSpecExposesNarrationAudio(t *testing.T) {
	props, ok := LlmSpec.JSONSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing from schema: %#v", LlmSpec.JSONSchema)
	}
	for _, name := range []string{"audio", "audio_mode", "narration_volume", "background_volume"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("expected %s property", name)
		}
	}
}

func TestBuildAttachAudioArgsPadsNarrationToVideoDuration(t *testing.T) {
	args := buildAttachAudioArgs("in.mp4", "voice.mp3", "out.mp4", 12.5, false, 1.0, 0.25)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "apad") || !strings.Contains(joined, "atrim=0:12.500") {
		t.Fatalf("expected padded/trimmed narration filter, got %q", joined)
	}
	if !strings.Contains(joined, "-map [narration]") {
		t.Fatalf("expected narration audio map, got %q", joined)
	}
}
