package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIToolsNormalizeTypedNilSchemaParts(t *testing.T) {
	var nilProperties map[string]any
	var nilRequired []string
	spec := ToolSpec{
		Name: "enter_plan_mode",
		JSONSchema: map[string]any{
			"type":       "object",
			"properties": nilProperties,
			"required":   nilRequired,
		},
	}

	chatTools := toOpenAITools([]ToolSpec{spec})
	chatParams := chatTools[0]["function"].(map[string]any)["parameters"].(map[string]any)
	assertObjectSchemaParts(t, chatParams)
	assertNoSchemaNulls(t, chatTools)

	responsesTools := toOpenAIResponsesTools([]ToolSpec{spec})
	responsesParams := responsesTools[0]["parameters"].(map[string]any)
	assertObjectSchemaParts(t, responsesParams)
	assertNoSchemaNulls(t, responsesTools)
}

func assertObjectSchemaParts(t *testing.T, params map[string]any) {
	t.Helper()
	properties, ok := params["properties"].(map[string]any)
	if !ok || properties == nil {
		t.Fatalf("properties must be a JSON object: %#v", params["properties"])
	}
	required, ok := params["required"].([]string)
	if !ok || required == nil {
		t.Fatalf("required must be a JSON array: %#v", params["required"])
	}
}

func assertNoSchemaNulls(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, fragment := range []string{`"parameters":null`, `"properties":null`, `"required":null`} {
		if strings.Contains(encoded, fragment) {
			t.Fatalf("schema contains %s: %s", fragment, encoded)
		}
	}
}
