package audiotts

import "testing"

func TestAudioTTSSpecExposesSpeechInputs(t *testing.T) {
	props, ok := LlmSpec.JSONSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing from schema: %#v", LlmSpec.JSONSchema)
	}
	for _, name := range []string{"text", "voice", "model_selector", "response_format", "artifact_name"} {
		if _, ok := props[name]; !ok {
			t.Fatalf("expected %s property", name)
		}
	}
}
