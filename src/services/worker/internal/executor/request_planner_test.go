package executor

import (
	"testing"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/personas"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/subagentctl"
)

func TestPlanRequestFromRunContextReusesInheritedPromptCacheSnapshot(t *testing.T) {
	description := "echo tool"
	route := &routing.SelectedProviderRoute{Route: routing.ProviderRouteRule{ID: "route-1", Model: "anthropic^claude-sonnet-4-5"}}
	tools := []llm.ToolSpec{{Name: "echo", Description: &description}}
	parentBaseMessages := []llm.Message{
		{Role: "user", Content: []llm.ContentPart{{Text: "parent question"}}},
		{Role: "assistant", Content: []llm.ContentPart{{Text: "parent answer"}}},
	}
	currentBaseMessages := append(cloneMessages(parentBaseMessages), llm.Message{
		Role:    "user",
		Content: []llm.ContentPart{{Text: "child task"}},
	})
	rc := &pipeline.RunContext{
		AgentConfig: &pipeline.ResolvedAgentConfig{PromptCacheControl: "system_prompt"},
		PersonaDefinition: &personas.Definition{
			ID:        "researcher@1",
			CoreTools: []string{"echo"},
		},
		PromptAssembly: pipeline.PromptAssembly{
			Segments: []pipeline.PromptSegment{
				{
					Name:      "persona.system_prompt",
					Target:    pipeline.PromptTargetSystemPrefix,
					Role:      "system",
					Text:      "current recomputed system",
					Stability: pipeline.PromptStabilityStablePrefix,
				},
				{
					Name:      "runtime.local_now",
					Target:    pipeline.PromptTargetRuntimeTail,
					Role:      "user",
					Text:      "current local now",
					Stability: pipeline.PromptStabilityVolatileTail,
				},
			},
		},
		SelectedRoute: route,
		FinalSpecs:    tools,
		ReasoningMode: "high",
		InheritedPromptCacheSnapshot: &subagentctl.PromptCacheSnapshot{
			PersonaID:    "researcher@1",
			BaseMessages: parentBaseMessages,
			Tools: []llm.ToolSpec{{
				Name:        "echo",
				Description: &description,
				CacheHint:   &llm.CacheHint{Action: llm.CacheHintActionWrite, Scope: "global"},
			}},
			Model:         route.Route.Model,
			ReasoningMode: "high",
			PromptPlan: &llm.PromptPlan{
				SystemBlocks: []llm.PromptPlanBlock{{
					Name:          "persona.system_prompt",
					Target:        llm.PromptTargetSystemPrefix,
					Role:          "system",
					Text:          "frozen parent system",
					Stability:     llm.CacheStabilityStablePrefix,
					CacheEligible: true,
				}},
			},
		},
	}

	planned := planRequestFromRunContext(rc, requestPlannerInput{
		Model:            route.Route.Model,
		BaseMessages:     currentBaseMessages,
		PromptMode:       promptPlanModeFull,
		Tools:            tools,
		ReasoningMode:    "high",
		ApplyImageFilter: false,
	})

	if got := planned.Request.PromptPlan.SystemBlocks[0].Text; got != "frozen parent system" {
		t.Fatalf("expected frozen system block, got %q", got)
	}
	if got := planned.Request.Messages[0].Content[0].Text; got != "frozen parent system" {
		t.Fatalf("expected frozen system message, got %q", got)
	}
	if got := planned.Request.Messages[len(planned.Request.Messages)-1].Content[0].Text; got != "current local now" {
		t.Fatalf("expected current runtime tail, got %q", got)
	}
	if got := planned.Request.Messages[len(planned.Request.Messages)-2].Content[0].Text; got != "child task" {
		t.Fatalf("expected child suffix message, got %q", got)
	}
	if got := planned.CacheSafeSnapshot.PersonaID; got != "researcher@1" {
		t.Fatalf("unexpected cache snapshot persona id: %q", got)
	}
}

func TestBuildPromptPlan_StableMarker(t *testing.T) {
	rc := &pipeline.RunContext{
		AgentConfig: &pipeline.ResolvedAgentConfig{PromptCacheControl: "system_prompt"},
		PromptAssembly: pipeline.PromptAssembly{
			Segments: []pipeline.PromptSegment{
				{
					Name:      "persona.system_prompt",
					Target:    pipeline.PromptTargetSystemPrefix,
					Role:      "system",
					Text:      "test system",
					Stability: pipeline.PromptStabilityStablePrefix,
				},
			},
		},
	}

	messages := []llm.Message{
		{Role: "user", Content: []llm.ContentPart{{Text: "msg0"}}},
		{Role: "assistant", Content: []llm.ContentPart{{Text: "msg1"}}},
		{Role: "user", Content: []llm.ContentPart{{Text: "msg2"}}},
		{Role: "assistant", Content: []llm.ContentPart{{Text: "msg3"}}},
		{Role: "user", Content: []llm.ContentPart{{Text: "msg4"}}},
	}

	plan := buildPromptPlan(rc, promptPlanModeFull, messages, 2)

	if plan == nil {
		t.Fatal("expected non-nil prompt plan")
	}
	if !plan.MessageCache.StableMarkerEnabled {
		t.Error("expected StableMarkerEnabled to be true")
	}
	if plan.MessageCache.StableMarkerMessageIndex != 2 {
		t.Errorf("expected StableMarkerMessageIndex = 2, got %d", plan.MessageCache.StableMarkerMessageIndex)
	}
}

func TestBuildPromptPlan_NoTail(t *testing.T) {
	rc := &pipeline.RunContext{
		AgentConfig: &pipeline.ResolvedAgentConfig{PromptCacheControl: "system_prompt"},
		PromptAssembly: pipeline.PromptAssembly{
			Segments: []pipeline.PromptSegment{
				{
					Name:      "persona.system_prompt",
					Target:    pipeline.PromptTargetSystemPrefix,
					Role:      "system",
					Text:      "test system",
					Stability: pipeline.PromptStabilityStablePrefix,
				},
			},
		},
	}

	messages := []llm.Message{
		{Role: "user", Content: []llm.ContentPart{{Text: "msg0"}}},
		{Role: "assistant", Content: []llm.ContentPart{{Text: "msg1"}}},
		{Role: "user", Content: []llm.ContentPart{{Text: "msg2"}}},
		{Role: "assistant", Content: []llm.ContentPart{{Text: "msg3"}}},
		{Role: "user", Content: []llm.ContentPart{{Text: "msg4"}}},
	}

	plan := buildPromptPlan(rc, promptPlanModeFull, messages, 0)

	if plan == nil {
		t.Fatal("expected non-nil prompt plan")
	}
	if plan.MessageCache.StableMarkerEnabled {
		t.Error("expected StableMarkerEnabled to be false when tailCount is 0")
	}
}
