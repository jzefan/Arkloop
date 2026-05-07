package subagentctl

import (
	"encoding/json"
	"strings"
	"testing"

	"arkloop/services/worker/internal/llm"
)

func TestClonePromptCacheSnapshotPreservesEmptySchemaObjects(t *testing.T) {
	src := &PromptCacheSnapshot{
		Tools: []llm.ToolSpec{{
			Name: "enter_plan_mode",
			JSONSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		}},
	}

	cloned := ClonePromptCacheSnapshot(src)
	schema := cloned.Tools[0].JSONSchema

	properties, ok := schema["properties"].(map[string]any)
	if !ok || properties == nil {
		t.Fatalf("properties must stay an empty object: %#v", schema["properties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || required == nil {
		t.Fatalf("required must stay an empty array: %#v", schema["required"])
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, fragment := range []string{`"properties":null`, `"required":null`} {
		if strings.Contains(encoded, fragment) {
			t.Fatalf("schema contains %s: %s", fragment, encoded)
		}
	}
}

func TestClonePromptCacheSnapshotPreservesToolCallDisplayDescription(t *testing.T) {
	src := &PromptCacheSnapshot{
		BaseMessages: []llm.Message{{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ToolCallID:         "call_1",
				ToolName:           "exec_command",
				ArgumentsJSON:      map[string]any{"command": "git status"},
				DisplayDescription: "Checking status",
			}},
		}},
	}

	cloned := ClonePromptCacheSnapshot(src)
	if got := cloned.BaseMessages[0].ToolCalls[0].DisplayDescription; got != "Checking status" {
		t.Fatalf("expected display description to survive clone, got %q", got)
	}
}
