package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICodexResponsesGateway_SendsBackendRequestAndCompletes(t *testing.T) {
	t.Setenv("ARKLOOP_OUTBOUND_ALLOW_LOOPBACK_HTTP", "true")

	var captured map[string]any
	var gotHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotHeaders = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(openAISDKSSE([]string{
			`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
		})))
	}))
	defer server.Close()

	gateway := NewOpenAICodexResponsesGateway(OpenAICodexResponsesGatewayConfig{
		Transport: TransportConfig{
			APIKey:  "oauth-access",
			BaseURL: server.URL + "/backend-api",
			DefaultHeaders: map[string]string{
				"chatgpt-account-id": "acc_123",
			},
		},
		Protocol: OpenAIProtocolConfig{AdvancedPayloadJSON: map[string]any{}},
	})

	var completed *StreamRunCompleted
	maxOutputTokens := 4096
	temperature := 0.7
	if err := gateway.Stream(context.Background(), Request{
		Model:           "gpt-5.3-codex",
		MaxOutputTokens: &maxOutputTokens,
		Temperature:     &temperature,
		Messages: []Message{
			{Role: "system", Content: []TextPart{{Text: "policy"}}},
			{Role: "user", Content: []TextPart{{Text: "hello"}}},
		},
	}, func(event StreamEvent) error {
		if ev, ok := event.(StreamRunCompleted); ok {
			completed = &ev
		}
		return nil
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if gotHeaders.Get("Authorization") != "Bearer oauth-access" {
		t.Fatalf("unexpected authorization: %q", gotHeaders.Get("Authorization"))
	}
	if gotHeaders.Get("chatgpt-account-id") != "acc_123" {
		t.Fatalf("unexpected account header: %q", gotHeaders.Get("chatgpt-account-id"))
	}
	if gotHeaders.Get("OpenAI-Beta") != "responses=experimental" || gotHeaders.Get("originator") != "pi" {
		t.Fatalf("unexpected codex headers: %#v", gotHeaders)
	}
	if gotHeaders.Get("Accept") != "text/event-stream" || !strings.Contains(gotHeaders.Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected content headers: %#v", gotHeaders)
	}
	if !strings.HasPrefix(gotHeaders.Get("User-Agent"), "pi (") {
		t.Fatalf("unexpected user agent: %q", gotHeaders.Get("User-Agent"))
	}

	if captured["model"] != "gpt-5.3-codex" || captured["instructions"] != "policy" || captured["stream"] != true {
		t.Fatalf("unexpected payload: %#v", captured)
	}
	if captured["store"] != false || captured["tool_choice"] != "auto" || captured["parallel_tool_calls"] != true {
		t.Fatalf("missing codex payload defaults: %#v", captured)
	}
	if _, exists := captured["max_output_tokens"]; exists {
		t.Fatalf("codex backend does not accept max_output_tokens: %#v", captured)
	}
	if _, exists := captured["temperature"]; exists {
		t.Fatalf("codex backend does not accept temperature: %#v", captured)
	}
	text, _ := captured["text"].(map[string]any)
	if text["verbosity"] != "medium" {
		t.Fatalf("unexpected text verbosity: %#v", captured["text"])
	}
	include, _ := captured["include"].([]any)
	if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
		t.Fatalf("unexpected include: %#v", captured["include"])
	}
	if completed == nil || completed.AssistantMessage == nil || VisibleMessageText(*completed.AssistantMessage) != "ok" {
		t.Fatalf("unexpected completion: %#v", completed)
	}
	if completed.Usage == nil || completed.Usage.InputTokens == nil || *completed.Usage.InputTokens != 1 {
		t.Fatalf("unexpected usage: %#v", completed.Usage)
	}
}

func TestOpenAICodexResponsesGateway_StripsUnsupportedOpenAIFields(t *testing.T) {
	t.Setenv("ARKLOOP_OUTBOUND_ALLOW_LOOPBACK_HTTP", "true")

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(openAISDKSSE([]string{
			`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}}`,
		})))
	}))
	defer server.Close()

	gateway := NewOpenAICodexResponsesGateway(OpenAICodexResponsesGatewayConfig{
		Transport: TransportConfig{
			APIKey:  "oauth-access",
			BaseURL: server.URL + "/backend-api/codex",
			DefaultHeaders: map[string]string{
				"chatgpt-account-id": "acc_123",
			},
		},
		Protocol: OpenAIProtocolConfig{AdvancedPayloadJSON: map[string]any{
			"top_p":               0.9,
			"metadata":            map[string]any{"debug": true},
			"prompt_cache_key":    "conversation-1",
			"client_metadata":     map[string]any{"installation_id": "install_1"},
			"parallel_tool_calls": false,
		}},
	})

	temperature := 0.7
	maxOutputTokens := 4096
	if err := gateway.Stream(context.Background(), Request{
		Model:           "gpt-5.3-codex",
		Temperature:     &temperature,
		MaxOutputTokens: &maxOutputTokens,
		Messages:        []Message{{Role: "user", Content: []TextPart{{Text: "hello"}}}},
	}, func(event StreamEvent) error { return nil }); err != nil {
		t.Fatalf("stream: %v", err)
	}

	for _, key := range []string{"temperature", "max_output_tokens", "top_p", "metadata"} {
		if _, exists := captured[key]; exists {
			t.Fatalf("unsupported key %s reached codex payload: %#v", key, captured)
		}
	}
	if captured["prompt_cache_key"] != "conversation-1" || captured["client_metadata"] == nil {
		t.Fatalf("expected codex allowlisted metadata to remain: %#v", captured)
	}
	if captured["parallel_tool_calls"] != false {
		t.Fatalf("expected explicit parallel_tool_calls to remain: %#v", captured)
	}
}

func TestOpenAICodexResponsesGateway_EmitsToolCallFromCompletedResponse(t *testing.T) {
	t.Setenv("ARKLOOP_OUTBOUND_ALLOW_LOOPBACK_HTTP", "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(openAISDKSSE([]string{
			`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"function_call","call_id":"call_1","name":"echo","arguments":"{\"text\":\"hi\"}"}],"usage":{"input_tokens":1,"output_tokens":2}}}`,
		})))
	}))
	defer server.Close()

	gateway := NewOpenAICodexResponsesGateway(OpenAICodexResponsesGatewayConfig{
		Transport: TransportConfig{
			APIKey:  "oauth-access",
			BaseURL: server.URL + "/backend-api/codex",
			DefaultHeaders: map[string]string{
				"chatgpt-account-id": "acc_123",
			},
		},
		Protocol: OpenAIProtocolConfig{AdvancedPayloadJSON: map[string]any{}},
	})

	var tool *ToolCall
	var completed *StreamRunCompleted
	if err := gateway.Stream(context.Background(), Request{
		Model:    "gpt-5.3-codex",
		Messages: []Message{{Role: "user", Content: []TextPart{{Text: "hello"}}}},
		Tools: []ToolSpec{{
			Name:       "echo",
			JSONSchema: map[string]any{"type": "object"},
		}},
	}, func(event StreamEvent) error {
		switch ev := event.(type) {
		case ToolCall:
			tool = &ev
		case StreamRunCompleted:
			completed = &ev
		}
		return nil
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if tool == nil || tool.ToolCallID != "call_1" || tool.ToolName != "echo" || tool.ArgumentsJSON["text"] != "hi" {
		t.Fatalf("unexpected tool call: %#v", tool)
	}
	if completed == nil {
		t.Fatalf("expected completion")
	}
}

func TestOpenAICodexResponsesGateway_MapsResponseDoneToCompleted(t *testing.T) {
	t.Setenv("ARKLOOP_OUTBOUND_ALLOW_LOOPBACK_HTTP", "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(openAISDKSSE([]string{
			`{"type":"response.done","response":{"id":"resp_1","status":"succeeded","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}}`,
		})))
	}))
	defer server.Close()

	gateway := NewOpenAICodexResponsesGateway(OpenAICodexResponsesGatewayConfig{
		Transport: TransportConfig{
			APIKey:  "oauth-access",
			BaseURL: server.URL + "/backend-api/codex",
			DefaultHeaders: map[string]string{
				"chatgpt-account-id": "acc_123",
			},
		},
		Protocol: OpenAIProtocolConfig{AdvancedPayloadJSON: map[string]any{}},
	})

	var completed *StreamRunCompleted
	if err := gateway.Stream(context.Background(), Request{
		Model:    "gpt-5.3-codex",
		Messages: []Message{{Role: "user", Content: []TextPart{{Text: "hello"}}}},
	}, func(event StreamEvent) error {
		if ev, ok := event.(StreamRunCompleted); ok {
			completed = &ev
		}
		return nil
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if completed == nil || completed.AssistantMessage == nil || VisibleMessageText(*completed.AssistantMessage) != "done" {
		t.Fatalf("unexpected completion: %#v", completed)
	}
}

func TestOpenAICodexResponsesGateway_EmitsFunctionCallArgumentDeltas(t *testing.T) {
	t.Setenv("ARKLOOP_OUTBOUND_ALLOW_LOOPBACK_HTTP", "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(openAISDKSSE([]string{
			`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"echo"}}`,
			`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"text\":\""}`,
			`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"ok\"}"}`,
			`{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"echo","arguments":"{\"text\":\"ok\"}"}]}}`,
		})))
	}))
	defer server.Close()

	gateway := NewOpenAICodexResponsesGateway(OpenAICodexResponsesGatewayConfig{
		Transport: TransportConfig{
			APIKey:  "oauth-access",
			BaseURL: server.URL + "/backend-api/codex",
			DefaultHeaders: map[string]string{
				"chatgpt-account-id": "acc_123",
			},
		},
		Protocol: OpenAIProtocolConfig{AdvancedPayloadJSON: map[string]any{}},
	})

	var deltas []ToolCallArgumentDelta
	var tool *ToolCall
	if err := gateway.Stream(context.Background(), Request{
		Model:    "gpt-5.3-codex",
		Messages: []Message{{Role: "user", Content: []TextPart{{Text: "hello"}}}},
		Tools: []ToolSpec{{
			Name:       "echo",
			JSONSchema: map[string]any{"type": "object"},
		}},
	}, func(event StreamEvent) error {
		switch ev := event.(type) {
		case ToolCallArgumentDelta:
			deltas = append(deltas, ev)
		case ToolCall:
			tool = &ev
		}
		return nil
	}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if len(deltas) != 2 || deltas[0].ToolCallID != "call_1" || deltas[0].ToolName != "echo" || deltas[0].ArgumentsDelta+deltas[1].ArgumentsDelta != `{"text":"ok"}` {
		t.Fatalf("unexpected argument deltas: %#v", deltas)
	}
	if tool == nil || tool.ToolCallID != "call_1" || tool.ToolName != "echo" || tool.ArgumentsJSON["text"] != "ok" {
		t.Fatalf("unexpected tool call: %#v", tool)
	}
}
