package llm

import (
	"reflect"
	"sync"
	"testing"
)

func TestQuirkMatch_EchoReasoningContent(t *testing.T) {
	q := openAIQuirks[0]
	if q.ID != QuirkEchoReasoningContent {
		t.Fatalf("expected echo_reasoning_content, got %s", q.ID)
	}
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"deepseek_real", 400, `{"error":{"message":"Error from provider (DeepSeek): The reasoning_content in the thinking mode must be passed back to the API."}}`, true},
		{"moonshot_missing", 400, `{"error":{"message":"reasoning_content is missing when thinking is enabled"}}`, true},
		{"wrong_status", 500, `reasoning_content must be passed back`, false},
		{"missing_phrase", 400, `reasoning_content is invalid`, false},
		{"missing_field", 400, `must be passed back`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := q.Match(tc.status, tc.body)
			if got != tc.want {
				t.Fatalf("Match(%d,%q)=%v want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestQuirkMatch_StripUnsignedThinking(t *testing.T) {
	q := anthropicQuirks[0]
	if q.ID != QuirkStripUnsignedThinking {
		t.Fatalf("unexpected id %s", q.ID)
	}
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"real", 400, `{"error":{"message":"Invalid signature in thinking block"}}`, true},
		{"wrong_status", 200, `Invalid signature thinking`, false},
		{"missing_thinking", 400, `Invalid signature for token`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := q.Match(tc.status, tc.body); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestQuirkMatch_ForceTempOneOnThinking(t *testing.T) {
	q := anthropicQuirks[1]
	if q.ID != QuirkForceTempOneOnThink {
		t.Fatalf("unexpected id %s", q.ID)
	}
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"real", 400, `temperature may only be set to 1 when thinking is enabled`, true},
		{"wrong_status", 500, `temperature may only be set to 1`, false},
		{"missing_temp", 400, `value may only be set to 1`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := q.Match(tc.status, tc.body); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestQuirkApply_EchoReasoningContent(t *testing.T) {
	payload := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": "hello"},
			{"role": "assistant", "content": "x", "reasoning_content": "keep"},
		},
		"input": []any{
			map[string]any{"role": "assistant", "content": []any{}},
		},
	}
	applyEchoReasoningContent(payload)
	msgs := payload["messages"].([]map[string]any)
	if _, ok := msgs[0]["reasoning_content"]; ok {
		t.Fatalf("user must not get reasoning_content")
	}
	if msgs[1]["reasoning_content"] != "" {
		t.Fatalf("assistant should get empty reasoning_content, got %#v", msgs[1])
	}
	if msgs[2]["reasoning_content"] != "keep" {
		t.Fatalf("must not overwrite existing: %#v", msgs[2])
	}
	inputItem := payload["input"].([]any)[0].(map[string]any)
	if _, ok := inputItem["reasoning_content"]; ok {
		t.Fatalf("responses input must not get reasoning_content: %#v", inputItem)
	}
}

func TestQuirkApply_StripUnsignedThinking(t *testing.T) {
	payload := map[string]any{
		"messages": []map[string]any{
			{
				"role": "assistant",
				"content": []map[string]any{
					{"type": "thinking", "thinking": "drop"},
					{"type": "thinking", "thinking": "drop2", "signature": ""},
					{"type": "thinking", "thinking": "keep", "signature": "sig"},
					{"type": "text", "text": "answer"},
				},
			},
		},
	}
	applyStripUnsignedThinking(payload)
	content := payload["messages"].([]map[string]any)[0]["content"].([]map[string]any)
	if len(content) != 2 {
		t.Fatalf("expected 2 blocks left, got %#v", content)
	}
	if content[0]["thinking"] != "keep" || content[1]["type"] != "text" {
		t.Fatalf("unexpected content after strip: %#v", content)
	}
}

func TestQuirkApply_ForceTempOneOnThinking(t *testing.T) {
	temp := 0.7
	payload := map[string]any{
		"thinking":    map[string]any{"type": "enabled", "budget_tokens": 1024},
		"temperature": temp,
	}
	applyForceTempOneOnThinking(payload)
	if payload["temperature"] != 1.0 {
		t.Fatalf("expected temperature=1, got %#v", payload["temperature"])
	}

	disabled := map[string]any{"thinking": map[string]any{"type": "disabled"}, "temperature": 0.7}
	applyForceTempOneOnThinking(disabled)
	if disabled["temperature"] != 0.7 {
		t.Fatalf("must not change temperature when thinking disabled: %#v", disabled)
	}

	missing := map[string]any{"temperature": 0.5}
	applyForceTempOneOnThinking(missing)
	if missing["temperature"] != 0.5 {
		t.Fatalf("must not change without thinking: %#v", missing)
	}
}

func TestQuirkStore_Concurrent(t *testing.T) {
	store := NewQuirkStore()
	var wg sync.WaitGroup
	ids := []QuirkID{QuirkEchoReasoningContent, QuirkStripUnsignedThinking, QuirkForceTempOneOnThink}
	for _, id := range ids {
		id := id
		wg.Add(2)
		go func() { defer wg.Done(); store.Set(id) }()
		go func() { defer wg.Done(); _ = store.Has(id) }()
	}
	wg.Wait()
	for _, id := range ids {
		if !store.Has(id) {
			t.Fatalf("missing %s", id)
		}
	}
}

func TestQuirkStore_ApplyAll_OnlyActive(t *testing.T) {
	store := NewQuirkStore()
	payload := map[string]any{
		"messages": []map[string]any{{"role": "assistant", "content": "hi"}},
	}
	original := map[string]any{"messages": []map[string]any{{"role": "assistant", "content": "hi"}}}
	store.ApplyAll(payload, openAIQuirks)
	if !reflect.DeepEqual(payload, original) {
		t.Fatalf("inactive store must not modify payload")
	}
	store.Set(QuirkEchoReasoningContent)
	store.ApplyAll(payload, openAIQuirks)
	msgs := payload["messages"].([]map[string]any)
	if msgs[0]["reasoning_content"] != "" {
		t.Fatalf("after Set, reasoning_content must be added: %#v", msgs[0])
	}
}

func TestDetectQuirk(t *testing.T) {
	id, ok := detectQuirk(400, `reasoning_content must be passed back to the API`, openAIQuirks)
	if !ok || id != QuirkEchoReasoningContent {
		t.Fatalf("expected echo, got %s ok=%v", id, ok)
	}
	if _, ok := detectQuirk(200, `reasoning_content must be passed back`, openAIQuirks); ok {
		t.Fatalf("status 200 must not match")
	}
	id, ok = detectQuirk(400, `reasoning_content is missing because thinking is enabled`, openAIQuirks)
	if !ok || id != QuirkEchoReasoningContent {
		t.Fatalf("expected moonshot echo, got %s ok=%v", id, ok)
	}
	id, ok = detectQuirk(400, `Invalid signature in thinking block`, anthropicQuirks)
	if !ok || id != QuirkStripUnsignedThinking {
		t.Fatalf("expected strip, got %s ok=%v", id, ok)
	}
}
