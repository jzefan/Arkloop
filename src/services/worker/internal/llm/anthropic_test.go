package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToAnthropicMessages_ToolEnvelope(t *testing.T) {
	system, messages, err := toAnthropicMessages([]Message{
		{Role: "system", Content: []TextPart{{Text: "sys"}}},
		{
			Role:    "assistant",
			Content: []TextPart{{Text: ""}},
			ToolCalls: []ToolCall{
				{
					ToolCallID: "call_1",
					ToolName:   "web_search",
					ArgumentsJSON: map[string]any{
						"query": "hello",
					},
				},
			},
		},
		{
			Role: "tool",
			Content: []TextPart{{
				Text: `{"tool_call_id":"call_1","tool_name":"web_search","result":{"items":[{"title":"x"}]}}`,
			}},
		},
		{Role: "user", Content: []TextPart{{Text: "next"}}},
	})
	if err != nil {
		t.Fatalf("toAnthropicMessages failed: %v", err)
	}
	if system != "sys" {
		t.Fatalf("unexpected system: %q", system)
	}
	if len(messages) != 3 {
		t.Fatalf("unexpected messages len: %d", len(messages))
	}

	assistant := messages[0]
	if assistant["role"] != "assistant" {
		t.Fatalf("unexpected assistant role: %#v", assistant["role"])
	}
	rawBlocks, ok := assistant["content"].([]map[string]any)
	if !ok || len(rawBlocks) != 1 {
		t.Fatalf("unexpected assistant content: %#v", assistant["content"])
	}
	if rawBlocks[0]["type"] != "tool_use" {
		t.Fatalf("unexpected tool_use block: %#v", rawBlocks[0])
	}
	if rawBlocks[0]["id"] != "call_1" || rawBlocks[0]["name"] != "web_search" {
		t.Fatalf("unexpected tool_use id/name: %#v", rawBlocks[0])
	}
	input, ok := rawBlocks[0]["input"].(map[string]any)
	if !ok || input["query"] != "hello" {
		t.Fatalf("unexpected tool_use input: %#v", rawBlocks[0]["input"])
	}

	toolResult := messages[1]
	if toolResult["role"] != "user" {
		t.Fatalf("unexpected tool_result wrapper role: %#v", toolResult["role"])
	}
	rawToolResults, ok := toolResult["content"].([]map[string]any)
	if !ok || len(rawToolResults) != 1 {
		t.Fatalf("unexpected tool_result wrapper content: %#v", toolResult["content"])
	}
	if rawToolResults[0]["type"] != "tool_result" {
		t.Fatalf("unexpected tool_result block: %#v", rawToolResults[0])
	}
	if rawToolResults[0]["tool_use_id"] != "call_1" {
		t.Fatalf("unexpected tool_use_id: %#v", rawToolResults[0]["tool_use_id"])
	}
	content, ok := rawToolResults[0]["content"].(string)
	if !ok {
		t.Fatalf("unexpected tool_result content: %#v", rawToolResults[0]["content"])
	}
	var parsedContent map[string]any
	if err := json.Unmarshal([]byte(content), &parsedContent); err != nil {
		t.Fatalf("tool_result content not json: %v", err)
	}
	if _, ok := parsedContent["items"]; !ok {
		t.Fatalf("expected items in tool_result content, got %#v", parsedContent)
	}

	user := messages[2]
	if user["role"] != "user" {
		t.Fatalf("unexpected user role: %#v", user["role"])
	}
}

func TestParseAnthropicMessage_ToolUse(t *testing.T) {
	body := []byte(`{
  "id":"msg_test",
  "type":"message",
  "role":"assistant",
  "content":[
    {"type":"text","text":"ok"},
    {"type":"tool_use","id":"call_1","name":"web_search","input":{"query":"hello"}}
  ]
}`)

	content, toolCalls, err := parseAnthropicMessage(body)
	if err != nil {
		t.Fatalf("parseAnthropicMessage failed: %v", err)
	}
	if content != "ok" {
		t.Fatalf("unexpected content: %q", content)
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ToolCallID != "call_1" || toolCalls[0].ToolName != "web_search" {
		t.Fatalf("unexpected tool call: %#v", toolCalls[0])
	}
	if toolCalls[0].ArgumentsJSON["query"] != "hello" {
		t.Fatalf("unexpected tool call args: %#v", toolCalls[0].ArgumentsJSON)
	}
}

func TestAnthropicGateway_Stream_ToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"msg_test",
  "type":"message",
  "role":"assistant",
  "content":[
    {"type":"tool_use","id":"call_1","name":"web_search","input":{"query":"hello"}}
  ]
}`))
	}))
	t.Cleanup(server.Close)

	gateway := NewAnthropicGateway(AnthropicGatewayConfig{
		APIKey:          "test",
		BaseURL:         server.URL,
		EmitDebugEvents: false,
	})

	events := []StreamEvent{}
	err := gateway.Stream(context.Background(), Request{
		Model: "claude-test",
		Messages: []Message{
			{Role: "user", Content: []TextPart{{Text: "hi"}}},
		},
		Tools: []ToolSpec{
			{Name: "web_search", JSONSchema: map[string]any{"type": "object"}},
		},
	}, func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}

	var gotCall *ToolCall
	for _, item := range events {
		call, ok := item.(ToolCall)
		if !ok {
			continue
		}
		copied := call
		gotCall = &copied
		break
	}
	if gotCall == nil {
		t.Fatalf("expected tool call event, got %d events", len(events))
	}
	if gotCall.ToolCallID != "call_1" || gotCall.ToolName != "web_search" || gotCall.ArgumentsJSON["query"] != "hello" {
		t.Fatalf("unexpected tool call: %#v", gotCall)
	}

	if _, ok := events[len(events)-1].(StreamRunCompleted); !ok {
		t.Fatalf("expected StreamRunCompleted as last event, got %T", events[len(events)-1])
	}
}
