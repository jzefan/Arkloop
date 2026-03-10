package pipeline_test

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/personas"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/tools"
)

func TestPersonaResolutionPreferredCredentialSet(t *testing.T) {
	credName := "my-anthropic"
	reg := buildPersonaRegistry(t, personas.Definition{
		ID:                  "test-persona",
		Version:             "1",
		Title:               "Test Persona",
		SoulMD:              "persona soul",
		PromptMD:            "# test",
		ExecutorType:        "agent.simple",
		ExecutorConfig:      map[string]any{},
		PreferredCredential: &credName,
	})
	mw := pipeline.NewPersonaResolutionMiddleware(
		func() *personas.Registry { return reg },
		nil, data.RunsRepository{}, data.RunEventsRepository{}, nil,
	)

	rc := &pipeline.RunContext{InputJSON: map[string]any{"persona_id": "test-persona"}}

	var capturedCredName string
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		capturedCredName = rc.PreferredCredentialName
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCredName != credName {
		t.Fatalf("expected PreferredCredentialName %q, got %q", credName, capturedCredName)
	}
}

func TestPersonaResolutionNoPreferredCredentialEmpty(t *testing.T) {
	reg := buildPersonaRegistry(t, personas.Definition{
		ID:             "test-persona",
		Version:        "1",
		Title:          "Test Persona",
		SoulMD:         "persona soul",
		PromptMD:       "# test",
		ExecutorType:   "agent.simple",
		ExecutorConfig: map[string]any{},
	})
	mw := pipeline.NewPersonaResolutionMiddleware(
		func() *personas.Registry { return reg },
		nil, data.RunsRepository{}, data.RunEventsRepository{}, nil,
	)

	rc := &pipeline.RunContext{InputJSON: map[string]any{"persona_id": "test-persona"}}

	var credName string
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		credName = rc.PreferredCredentialName
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credName != "" {
		t.Fatalf("expected PreferredCredentialName empty, got %q", credName)
	}
}

func TestPersonaResolutionUserRouteIDNotAffectedByPersonaCredential(t *testing.T) {
	personaCred := "my-anthropic"
	userRouteID := "openai-gpt4"
	reg := buildPersonaRegistry(t, personas.Definition{
		ID:                  "test-persona",
		Version:             "1",
		Title:               "Test Persona",
		SoulMD:              "persona soul",
		PromptMD:            "# test",
		ExecutorType:        "agent.simple",
		ExecutorConfig:      map[string]any{},
		PreferredCredential: &personaCred,
	})
	mw := pipeline.NewPersonaResolutionMiddleware(
		func() *personas.Registry { return reg },
		nil, data.RunsRepository{}, data.RunEventsRepository{}, nil,
	)

	rc := &pipeline.RunContext{
		InputJSON: map[string]any{
			"persona_id": "test-persona",
			"route_id":   userRouteID,
		},
	}

	var capturedRouteID any
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		capturedRouteID = rc.InputJSON["route_id"]
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRouteID != userRouteID {
		t.Fatalf("expected user route_id %q to be preserved, got %v", userRouteID, capturedRouteID)
	}
}

func TestPersonaResolutionLoadsModelReasoningAndPromptCache(t *testing.T) {
	model := "demo-cred^gpt-5-mini"
	reg := buildPersonaRegistry(t, personas.Definition{
		ID:                 "test-persona",
		Version:            "1",
		Title:              "Test Persona",
		SoulMD:             "persona soul",
		PromptMD:           "system prompt",
		ExecutorType:       "agent.simple",
		ExecutorConfig:     map[string]any{},
		Model:              &model,
		ReasoningMode:      "high",
		PromptCacheControl: "system_prompt",
	})
	mw := pipeline.NewPersonaResolutionMiddleware(
		func() *personas.Registry { return reg },
		nil, data.RunsRepository{}, data.RunEventsRepository{}, nil,
	)

	rc := &pipeline.RunContext{InputJSON: map[string]any{"persona_id": "test-persona"}}

	var gotConfig *pipeline.ResolvedAgentConfig
	var gotSystemPrompt string
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gotConfig = rc.AgentConfig
		gotSystemPrompt = rc.SystemPrompt
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotConfig == nil {
		t.Fatal("expected AgentConfig to be populated from persona")
	}
	if gotConfig.Model == nil || *gotConfig.Model != model {
		t.Fatalf("unexpected model: %#v", gotConfig.Model)
	}
	if gotConfig.ReasoningMode != "high" {
		t.Fatalf("unexpected reasoning_mode: %q", gotConfig.ReasoningMode)
	}
	if gotConfig.PromptCacheControl != "system_prompt" {
		t.Fatalf("unexpected prompt_cache_control: %q", gotConfig.PromptCacheControl)
	}
	if gotSystemPrompt != "persona soul\n\nsystem prompt" {
		t.Fatalf("unexpected system prompt: %q", gotSystemPrompt)
	}
}

func TestPersonaResolutionAppliesBudgets(t *testing.T) {
	reg := buildPersonaRegistry(t, personas.Definition{
		ID:             "p1",
		Version:        "1",
		Title:          "Test",
		SoulMD:         "persona soul",
		PromptMD:       "test",
		ExecutorType:   "agent.simple",
		ExecutorConfig: map[string]any{},
		Budgets: personas.Budgets{
			ReasoningIterations:    intPtr(4),
			ToolContinuationBudget: intPtr(12),
			MaxOutputTokens:        intPtr(900),
			Temperature:            floatPtr(0.3),
			TopP:                   floatPtr(0.8),
		},
	})
	mw := pipeline.NewPersonaResolutionMiddleware(
		func() *personas.Registry { return reg },
		nil, data.RunsRepository{}, data.RunEventsRepository{}, nil,
	)

	rc := &pipeline.RunContext{
		InputJSON:                     map[string]any{"persona_id": "p1"},
		AgentReasoningIterationsLimit: 9,
		ToolContinuationBudgetLimit:   18,
	}

	var (
		gotReasoningIterations int
		gotToolContinuation    int
		gotMaxOutputTokens     *int
		gotTemperature         *float64
		gotTopP                *float64
	)
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gotReasoningIterations = rc.ReasoningIterations
		gotToolContinuation = rc.ToolContinuationBudget
		gotMaxOutputTokens = rc.MaxOutputTokens
		gotTemperature = rc.Temperature
		gotTopP = rc.TopP
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReasoningIterations != 4 {
		t.Fatalf("unexpected reasoning iterations: %d", gotReasoningIterations)
	}
	if gotToolContinuation != 12 {
		t.Fatalf("unexpected tool continuation budget: %d", gotToolContinuation)
	}
	if gotMaxOutputTokens == nil || *gotMaxOutputTokens != 900 {
		t.Fatalf("unexpected max output tokens: %#v", gotMaxOutputTokens)
	}
	if gotTemperature == nil || *gotTemperature != 0.3 {
		t.Fatalf("unexpected temperature: %#v", gotTemperature)
	}
	if gotTopP == nil || *gotTopP != 0.8 {
		t.Fatalf("unexpected top_p: %#v", gotTopP)
	}
}

func TestPersonaResolutionToolAllowlistAndDenylist(t *testing.T) {
	registry := tools.NewRegistry()
	for _, spec := range []tools.AgentToolSpec{
		{Name: "tool_a", Version: "1", Description: "a", RiskLevel: tools.RiskLevelLow},
		{Name: "tool_b", Version: "1", Description: "b", RiskLevel: tools.RiskLevelLow},
		{Name: "tool_c", Version: "1", Description: "c", RiskLevel: tools.RiskLevelLow},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register tool: %v", err)
		}
	}

	reg := buildPersonaRegistry(t, personas.Definition{
		ID:             "p1",
		Version:        "1",
		Title:          "Test",
		PromptMD:       "test",
		ExecutorType:   "agent.simple",
		ExecutorConfig: map[string]any{},
		ToolAllowlist:  []string{"tool_b", "tool_c"},
		ToolDenylist:   []string{"tool_c"},
	})
	mw := pipeline.NewPersonaResolutionMiddleware(
		func() *personas.Registry { return reg },
		nil, data.RunsRepository{}, data.RunEventsRepository{}, nil,
	)

	rc := &pipeline.RunContext{
		InputJSON: map[string]any{"persona_id": "p1"},
		AllowlistSet: map[string]struct{}{
			"tool_a": {},
			"tool_b": {},
			"tool_c": {},
		},
		ToolRegistry: registry,
	}

	var gotAllowlist map[string]struct{}
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gotAllowlist = rc.AllowlistSet
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := gotAllowlist["tool_b"]; !ok {
		t.Fatalf("expected tool_b in allowlist, got %v", gotAllowlist)
	}
	if _, ok := gotAllowlist["tool_a"]; ok {
		t.Fatalf("tool_a should not be in persona allowlist, got %v", gotAllowlist)
	}
	if _, ok := gotAllowlist["tool_c"]; ok {
		t.Fatalf("tool_c should be removed by denylist, got %v", gotAllowlist)
	}
}

func buildPersonaRegistry(t *testing.T, defs ...personas.Definition) *personas.Registry {
	t.Helper()
	reg := personas.NewRegistry()
	for _, def := range defs {
		if err := reg.Register(def); err != nil {
			t.Fatalf("register persona failed: %v", err)
		}
	}
	return reg
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func strPtr(v string) *string { return &v }
