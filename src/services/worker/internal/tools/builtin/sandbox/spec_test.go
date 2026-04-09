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

func TestExecCommandLlmSpecUsesProcessModesOnly(t *testing.T) {
	schema, ok := ExecCommandLlmSpec.JSONSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected exec_command properties schema, got %#v", ExecCommandLlmSpec.JSONSchema)
	}
	modeField, ok := schema["mode"].(map[string]any)
	if !ok {
		t.Fatalf("expected mode field, got %#v", schema["mode"])
	}
	enumValues, ok := modeField["enum"].([]string)
	if !ok {
		t.Fatalf("expected mode enum, got %#v", modeField["enum"])
	}
	want := []string{"buffered", "follow", "stdin", "pty"}
	if len(enumValues) != len(want) {
		t.Fatalf("unexpected mode enum: %#v", enumValues)
	}
	for i, value := range want {
		if enumValues[i] != value {
			t.Fatalf("unexpected mode enum order: %#v", enumValues)
		}
	}
	for _, legacy := range []string{"session_mode", "session_ref", "from_session_ref", "share_scope", "yield_time_ms", "background"} {
		if _, exists := schema[legacy]; exists {
			t.Fatalf("exec_command llm spec should not expose %s: %#v", legacy, schema)
		}
	}
}

func TestContinueProcessLlmSpecUsesProcessRefCursorAndWait(t *testing.T) {
	schema, ok := ContinueProcessLlmSpec.JSONSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected continue_process properties schema, got %#v", ContinueProcessLlmSpec.JSONSchema)
	}
	for _, required := range []string{"process_ref", "cursor", "wait_ms", "stdin_text", "input_seq", "close_stdin"} {
		if _, exists := schema[required]; !exists {
			t.Fatalf("continue_process should expose %s: %#v", required, schema)
		}
	}
	for _, legacy := range []string{"session_ref", "chars", "yield_time_ms"} {
		if _, exists := schema[legacy]; exists {
			t.Fatalf("continue_process should not expose %s: %#v", legacy, schema)
		}
	}
}
