package sandbox

import "testing"

func TestBrowserLlmSpecHidesSessionRef(t *testing.T) {
	schema, ok := BrowserLlmSpec.JSONSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected browser properties schema, got %#v", BrowserLlmSpec.JSONSchema)
	}
	if _, exists := schema["session_ref"]; exists {
		t.Fatalf("browser llm spec should not expose session_ref: %#v", schema)
	}
	if len(schema) != 2 {
		t.Fatalf("browser llm spec should only expose command and yield_time_ms, got %#v", schema)
	}
}
