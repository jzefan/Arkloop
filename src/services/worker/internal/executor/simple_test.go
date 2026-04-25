package executor

import (
	"context"
	"strings"
	"testing"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/tools"
	"github.com/google/uuid"
)

func buildMinimalRC(gateway llm.Gateway, systemPrompt string, agentConfig *pipeline.ResolvedAgentConfig, advance map[string]any) *pipeline.RunContext {
	rc := &pipeline.RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		TraceID:     "test-trace",
		Gateway:     gateway,
		Messages:    []llm.Message{},
		AgentConfig: agentConfig,
		SelectedRoute: &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:           "default",
				Model:        "stub",
				AdvancedJSON: advance,
			},
		},
		ReasoningIterations:    5,
		ToolContinuationBudget: 32,
		InputJSON:              map[string]any{},
		ToolBudget:             map[string]any{},
		PerToolSoftLimits:      tools.DefaultPerToolSoftLimits(),
		FinalSpecs:             []llm.ToolSpec{},
	}
	if strings.TrimSpace(systemPrompt) != "" {
		rc.PromptAssembly.Append(pipeline.PromptSegment{
			Name:          "test.system",
			Target:        pipeline.PromptTargetSystemPrefix,
			Role:          "system",
			Text:          systemPrompt,
			Stability:     pipeline.PromptStabilityStablePrefix,
			CacheEligible: agentConfig != nil && agentConfig.PromptCacheControl == "system_prompt",
		})
		rc.SystemPrompt = rc.MaterializedSystemPrompt()
	}
	return rc
}

func TestSimpleExecutor_EmitsExpectedEvents(t *testing.T) {
	gateway := llm.NewAuxGateway(llm.AuxGatewayConfig{
		Enabled:    true,
		DeltaCount: 2,
	})

	ex := &SimpleExecutor{}
	emitter := events.NewEmitter("trace")
	rc := buildMinimalRC(gateway, "", nil, nil)

	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	deltaCount, completedCount := 0, 0
	for _, ev := range got {
		switch ev.Type {
		case "message.delta":
			deltaCount++
		case "run.completed":
			completedCount++
		}
	}
	if deltaCount != 2 {
		t.Fatalf("expected 2 message.delta, got %d", deltaCount)
	}
	if completedCount != 1 {
		t.Fatalf("expected 1 run.completed, got %d", completedCount)
	}
}

func TestSimpleExecutor_SystemPromptInjected(t *testing.T) {
	var capturedMessages []llm.Message
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			capturedMessages = append(capturedMessages, req.Messages...)
		},
	}

	ex := &SimpleExecutor{}
	emitter := events.NewEmitter("trace")
	rc := buildMinimalRC(gateway, "you are a helpful assistant", nil, nil)

	_ = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error { return nil })

	if len(capturedMessages) == 0 {
		t.Fatal("no messages captured")
	}
	first := capturedMessages[0]
	if first.Role != "system" {
		t.Fatalf("expected first message role=system, got %q", first.Role)
	}
	if len(first.Content) == 0 || first.Content[0].Text != "you are a helpful assistant" {
		t.Fatalf("unexpected system content: %+v", first.Content)
	}
	if first.Content[0].CacheControl != nil {
		t.Fatalf("cache_control should be nil when PromptCacheControl is not set")
	}
}

func TestSimpleExecutor_ClampsMaxOutputTokensToRouteCatalog(t *testing.T) {
	captured := 0
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			if req.MaxOutputTokens != nil {
				captured = *req.MaxOutputTokens
			}
		},
	}

	ex := &SimpleExecutor{}
	rc := buildMinimalRC(gateway, "", nil, map[string]any{
		"available_catalog": map[string]any{
			"max_output_tokens": float64(16384),
		},
	})
	requested := 32768
	rc.MaxOutputTokens = &requested

	if err := ex.Execute(context.Background(), rc, events.NewEmitter("trace"), func(ev events.RunEvent) error { return nil }); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if captured != 16384 {
		t.Fatalf("expected max_output_tokens clamped to 16384, got %d", captured)
	}
}

func TestSimpleExecutor_SystemPromptCacheControl(t *testing.T) {
	var capturedMessages []llm.Message
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			capturedMessages = append(capturedMessages, req.Messages...)
		},
	}

	ex := &SimpleExecutor{}
	emitter := events.NewEmitter("trace")
	agentConfig := &pipeline.ResolvedAgentConfig{PromptCacheControl: "system_prompt"}
	rc := buildMinimalRC(gateway, "cached prompt", agentConfig, nil)

	_ = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error { return nil })

	if len(capturedMessages) == 0 || capturedMessages[0].Role != "system" {
		t.Fatal("expected system message")
	}
	cc := capturedMessages[0].Content[0].CacheControl
	if cc == nil || *cc != "ephemeral" {
		t.Fatalf("expected cache_control=ephemeral, got %v", cc)
	}
}

func TestSimpleExecutor_RuntimePromptInjectedAsUserMessage(t *testing.T) {
	var capturedMessages []llm.Message
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			capturedMessages = append(capturedMessages, req.Messages...)
		},
	}

	ex := &SimpleExecutor{}
	emitter := events.NewEmitter("trace")
	rc := buildMinimalRC(gateway, "stable system", nil, nil)
	rc.AppendPromptSegment(pipeline.PromptSegment{
		Name:          "test.runtime",
		Target:        pipeline.PromptTargetRuntimeTail,
		Role:          "user",
		Text:          "[SYSTEM_RUNTIME_CONTEXT]\nUser Local Now: 2026-04-11 20:38:55 [UTC+8]\n[/SYSTEM_RUNTIME_CONTEXT]",
		Stability:     pipeline.PromptStabilityVolatileTail,
		CacheEligible: false,
	})
	rc.Messages = []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "hello"}}},
	}

	_ = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error { return nil })

	if len(capturedMessages) != 3 {
		t.Fatalf("expected system + user + runtime prompt, got %#v", capturedMessages)
	}
	if capturedMessages[0].Role != "system" || capturedMessages[0].Content[0].Text != "stable system" {
		t.Fatalf("unexpected system message: %#v", capturedMessages[0])
	}
	if capturedMessages[1].Role != "user" || capturedMessages[1].Content[0].Text != "hello" {
		t.Fatalf("unexpected original user message: %#v", capturedMessages[1])
	}
	if capturedMessages[2].Role != "user" || !strings.Contains(capturedMessages[2].Content[0].Text, "User Local Now:") {
		t.Fatalf("expected runtime prompt as trailing user message, got %#v", capturedMessages[2])
	}
}

func TestSimpleExecutor_NoSystemPromptWhenEmpty(t *testing.T) {
	var capturedMessages []llm.Message
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			capturedMessages = append(capturedMessages, req.Messages...)
		},
	}

	ex := &SimpleExecutor{}
	emitter := events.NewEmitter("trace")
	userMsg := llm.Message{Role: "user", Content: []llm.TextPart{{Text: "hello"}}}
	rc := buildMinimalRC(gateway, "   ", nil, nil)
	rc.Messages = []llm.Message{userMsg}

	_ = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error { return nil })

	if len(capturedMessages) == 0 {
		t.Fatal("expected at least one message")
	}
	if capturedMessages[0].Role == "system" {
		t.Fatal("should not inject system message when SystemPrompt is blank")
	}
}

func TestSimpleExecutor_HeartbeatWithCompactSnapshotSendsExpectedMessages(t *testing.T) {
	var capturedMessages []llm.Message
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			capturedMessages = append(capturedMessages, req.Messages...)
		},
	}

	ex := &SimpleExecutor{}
	emitter := events.NewEmitter("trace")
	rc := buildMinimalRC(gateway, "persona prompt", nil, nil)
	rc.InputJSON = map[string]any{"run_kind": "heartbeat", "heartbeat_interval_minutes": 30}
	rc.Messages = []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "[Context summary for continuation]\n<state_snapshot>\nexisting summary\n</state_snapshot>"}}},
		{Role: "user", Content: []llm.TextPart{{Text: "latest real user input"}}},
	}
	rc.ThreadMessageIDs = []uuid.UUID{uuid.Nil, uuid.New()}

	mw := pipeline.NewHeartbeatPrepareMiddleware()
	err := mw(context.Background(), rc, func(ctx context.Context, nextRC *pipeline.RunContext) error {
		return ex.Execute(ctx, nextRC, emitter, func(ev events.RunEvent) error { return nil })
	})
	if err != nil {
		t.Fatalf("heartbeat middleware + execute failed: %v", err)
	}

	systemCount := 0
	personaSystemSeen := false
	for _, msg := range capturedMessages {
		if msg.Role == "system" {
			systemCount++
			if len(msg.Content) > 0 && msg.Content[0].Text == "persona prompt" {
				personaSystemSeen = true
			}
		}
	}
	if systemCount < 1 {
		t.Fatalf("expected at least one system message, got %#v", capturedMessages)
	}
	if !personaSystemSeen {
		t.Fatalf("expected persona system prompt to be present, got %#v", capturedMessages)
	}
	var snapshotMsg *llm.Message
	var latestUserMsg *llm.Message
	var heartbeatMsg *llm.Message
	for i := range capturedMessages {
		msg := &capturedMessages[i]
		if msg.Role != "user" || len(msg.Content) == 0 {
			continue
		}
		text := msg.Content[0].Text
		switch {
		case text == "[Context summary for continuation]\n<state_snapshot>\nexisting summary\n</state_snapshot>":
			snapshotMsg = msg
		case text == "latest real user input":
			latestUserMsg = msg
		case strings.Contains(text, "[SYSTEM_HEARTBEAT_CHECK]"):
			heartbeatMsg = msg
		}
	}
	if snapshotMsg == nil {
		t.Fatalf("missing compact snapshot user message: %#v", capturedMessages)
	}
	if latestUserMsg == nil {
		t.Fatalf("missing latest user input message: %#v", capturedMessages)
	}
	if heartbeatMsg == nil {
		t.Fatalf("missing heartbeat check user message: %#v", capturedMessages)
	}
	for _, want := range []string{
		"[SYSTEM_HEARTBEAT_CHECK]",
		"interval_minutes: 30",
		"new_user_messages: 1",
		"[/SYSTEM_HEARTBEAT_CHECK]",
	} {
		if !strings.Contains(heartbeatMsg.Content[0].Text, want) {
			t.Fatalf("expected heartbeat message to contain %q, got %q", want, heartbeatMsg.Content[0].Text)
		}
	}
}

func TestNewSimpleExecutor_Factory(t *testing.T) {
	ex, err := NewSimpleExecutor(nil)
	if err != nil {
		t.Fatalf("factory with nil config failed: %v", err)
	}
	if ex == nil {
		t.Fatal("factory returned nil")
	}

	ex2, err := NewSimpleExecutor(map[string]any{"ignored": true})
	if err != nil {
		t.Fatalf("factory with config failed: %v", err)
	}
	if ex2 == nil {
		t.Fatal("factory returned nil")
	}
}

func TestSimpleExecutor_ImageFilter(t *testing.T) {
	gateway := &captureRequestGateway{
		onCapture: func(req llm.Request) {
			if len(req.Messages) != 1 {
				t.Fatalf("expected 1 message")
			}
			if req.Messages[0].Content[0].Kind() != messagecontent.PartTypeText {
				t.Fatalf("expected image part downgraded to text")
			}
		},
	}

	advance := map[string]any{
		"available_catalog": map[string]any{
			"input_modalities": []string{"text"},
		},
	}
	rc := buildMinimalRC(gateway, "", nil, advance)
	rc.Messages = []llm.Message{
		{
			Role: "user",
			Content: []llm.ContentPart{
				{Type: messagecontent.PartTypeImage, Attachment: &messagecontent.AttachmentRef{Filename: "photo.jpg"}},
			},
		},
	}

	ex := &SimpleExecutor{}
	_ = ex.Execute(context.Background(), rc, events.NewEmitter("trace"), func(ev events.RunEvent) error { return nil })
}

// captureRequestGateway 记录首次 Stream 调用的 Request，随后返回 run.completed。
type captureRequestGateway struct {
	onCapture func(req llm.Request)
}

func (g *captureRequestGateway) Stream(_ context.Context, req llm.Request, yield func(llm.StreamEvent) error) error {
	if g.onCapture != nil {
		g.onCapture(req)
	}
	return yield(llm.StreamRunCompleted{})
}
