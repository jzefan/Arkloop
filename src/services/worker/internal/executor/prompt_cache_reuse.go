package executor

import (
	"reflect"
	"strings"

	"arkloop/services/worker/internal/agent"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/subagentctl"
)

func personaIDFromRunContext(rc *pipeline.RunContext) string {
	if rc == nil || rc.PersonaDefinition == nil {
		return ""
	}
	return strings.TrimSpace(rc.PersonaDefinition.ID)
}

func promptCacheSnapshotFromRunContext(rc *pipeline.RunContext, baseMessages []llm.Message) *subagentctl.PromptCacheSnapshot {
	if rc == nil || rc.SelectedRoute == nil {
		return nil
	}
	planned := planRequestFromRunContext(rc, requestPlannerInput{
		Model:            rc.SelectedRoute.Route.Model,
		BaseMessages:     baseMessages,
		PromptMode:       promptPlanModeFull,
		Tools:            rc.FinalSpecs,
		MaxOutputTokens:  rc.MaxOutputTokens,
		Temperature:      rc.Temperature,
		ReasoningMode:    rc.ReasoningMode,
		ToolChoice:       rc.ToolChoice,
		ApplyImageFilter: true,
	})
	if planned.CacheSafeSnapshot == nil {
		return nil
	}
	snapshot := planned.CacheSafeSnapshot
	return subagentctl.ClonePromptCacheSnapshot(&subagentctl.PromptCacheSnapshot{
		PersonaID:       strings.TrimSpace(snapshot.PersonaID),
		BaseMessages:    cloneMessages(snapshot.BaseMessages),
		Messages:        cloneMessages(snapshot.Messages),
		Tools:           cloneToolSpecs(snapshot.Tools),
		Model:           strings.TrimSpace(snapshot.Model),
		MaxOutputTokens: cloneIntPtr(snapshot.MaxOutputTokens),
		Temperature:     cloneFloatPtr(snapshot.Temperature),
		ReasoningMode:   strings.TrimSpace(snapshot.ReasoningMode),
		ToolChoice:      cloneToolChoice(snapshot.ToolChoice),
		PromptPlan:      clonePromptPlan(snapshot.PromptPlan),
	})
}

func inheritedPromptCacheRequest(
	rc *pipeline.RunContext,
	input requestPlannerInput,
	baseMessages []llm.Message,
	tools []llm.ToolSpec,
) (llm.Request, *agent.CacheSafeSnapshot, bool) {
	snapshot := inheritedPromptCacheSnapshot(rc)
	if !canReuseInheritedPromptCache(rc, input, baseMessages, tools, snapshot) {
		return llm.Request{}, nil, false
	}

	plan := buildInheritedPromptPlan(snapshot, buildPromptPlan(rc, input.PromptMode, nil))
	messages := buildInheritedPromptMessages(snapshot, plan, baseMessages)
	if promptCacheEnabled(rc) && len(messages) > 0 {
		if plan == nil {
			plan = &llm.PromptPlan{}
		}
		lastIndex := len(messages) - 1
		plan.MessageCache = llm.MessageCachePlan{
			Enabled:                   true,
			MarkerMessageIndex:        lastIndex,
			ToolResultCacheCutIndex:   lastIndex,
			ToolResultCacheReferences: true,
		}
	}

	request := llm.Request{
		Model:           input.Model,
		Messages:        messages,
		Tools:           cloneToolSpecs(snapshot.Tools),
		MaxOutputTokens: cloneIntPtr(input.MaxOutputTokens),
		Temperature:     cloneFloatPtr(input.Temperature),
		ReasoningMode:   strings.TrimSpace(input.ReasoningMode),
		ToolChoice:      cloneToolChoice(input.ToolChoice),
		PromptPlan:      plan,
	}
	return request, buildCacheSafeSnapshot(rc, baseMessages, request), true
}

func inheritedPromptCacheSnapshot(rc *pipeline.RunContext) *subagentctl.PromptCacheSnapshot {
	if rc == nil {
		return nil
	}
	return subagentctl.ClonePromptCacheSnapshot(rc.InheritedPromptCacheSnapshot)
}

func canReuseInheritedPromptCache(
	rc *pipeline.RunContext,
	input requestPlannerInput,
	baseMessages []llm.Message,
	tools []llm.ToolSpec,
	snapshot *subagentctl.PromptCacheSnapshot,
) bool {
	if input.PromptMode != promptPlanModeFull || snapshot == nil || snapshot.PromptPlan == nil {
		return false
	}
	if strings.TrimSpace(snapshot.PersonaID) == "" || strings.TrimSpace(snapshot.PersonaID) != personaIDFromRunContext(rc) {
		return false
	}
	if strings.TrimSpace(snapshot.Model) == "" || strings.TrimSpace(snapshot.Model) != strings.TrimSpace(input.Model) {
		return false
	}
	if strings.TrimSpace(snapshot.ReasoningMode) != strings.TrimSpace(input.ReasoningMode) {
		return false
	}
	if !intPtrEqual(snapshot.MaxOutputTokens, input.MaxOutputTokens) {
		return false
	}
	if !floatPtrEqual(snapshot.Temperature, input.Temperature) {
		return false
	}
	if !reflect.DeepEqual(snapshot.ToolChoice, input.ToolChoice) {
		return false
	}
	if !reflect.DeepEqual(snapshot.Tools, tools) {
		return false
	}
	if len(baseMessages) < len(snapshot.BaseMessages) {
		return false
	}
	for i := range snapshot.BaseMessages {
		if !reflect.DeepEqual(snapshot.BaseMessages[i].ToJSON(), baseMessages[i].ToJSON()) {
			return false
		}
	}
	return true
}

func buildInheritedPromptPlan(
	snapshot *subagentctl.PromptCacheSnapshot,
	current *llm.PromptPlan,
) *llm.PromptPlan {
	base := clonePromptPlan(snapshot.PromptPlan)
	if base == nil {
		base = &llm.PromptPlan{}
	}
	base.MessageBlocks = append(
		filterPromptPlanBlocks(snapshot.PromptPlan.MessageBlocks, llm.PromptTargetConversationPrefix),
		filterPromptPlanBlocks(nilIfNil(current).MessageBlocks, llm.PromptTargetRuntimeTail)...,
	)
	if current != nil {
		base.MessageCache = current.MessageCache
	}
	return base
}

func buildInheritedPromptMessages(
	snapshot *subagentctl.PromptCacheSnapshot,
	plan *llm.PromptPlan,
	baseMessages []llm.Message,
) []llm.Message {
	suffix := cloneMessages(baseMessages[len(snapshot.BaseMessages):])
	systemMessages := promptPlanBlocksToMessages(nilIfNil(plan).SystemBlocks)
	conversationPrefix := promptPlanBlocksToMessages(filterPromptPlanBlocks(nilIfNil(plan).MessageBlocks, llm.PromptTargetConversationPrefix))
	runtimeTail := promptPlanBlocksToMessages(filterPromptPlanBlocks(nilIfNil(plan).MessageBlocks, llm.PromptTargetRuntimeTail))

	messages := make([]llm.Message, 0, len(systemMessages)+len(conversationPrefix)+len(snapshot.BaseMessages)+len(suffix)+len(runtimeTail))
	messages = append(messages, systemMessages...)
	messages = append(messages, conversationPrefix...)
	messages = append(messages, cloneMessages(snapshot.BaseMessages)...)
	messages = append(messages, suffix...)
	messages = append(messages, runtimeTail...)
	return messages
}

func promptPlanBlocksToMessages(blocks []llm.PromptPlanBlock) []llm.Message {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]llm.Message, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		part := llm.TextPart{Text: text}
		if block.CacheEligible {
			cacheControl := "ephemeral"
			part.CacheControl = &cacheControl
		}
		role := strings.TrimSpace(block.Role)
		if role == "" {
			role = "user"
		}
		out = append(out, llm.Message{
			Role:    role,
			Content: []llm.ContentPart{part},
		})
	}
	return out
}

func filterPromptPlanBlocks(blocks []llm.PromptPlanBlock, target string) []llm.PromptPlanBlock {
	if len(blocks) == 0 {
		return nil
	}
	filtered := make([]llm.PromptPlanBlock, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Target) != strings.TrimSpace(target) {
			continue
		}
		filtered = append(filtered, block)
	}
	return filtered
}

func buildCacheSafeSnapshot(rc *pipeline.RunContext, baseMessages []llm.Message, req llm.Request) *agent.CacheSafeSnapshot {
	return &agent.CacheSafeSnapshot{
		PersonaID:       personaIDFromRunContext(rc),
		BaseMessages:    cloneMessages(baseMessages),
		Model:           req.Model,
		Messages:        cloneMessages(req.Messages),
		Tools:           cloneToolSpecs(req.Tools),
		MaxOutputTokens: cloneIntPtr(req.MaxOutputTokens),
		Temperature:     cloneFloatPtr(req.Temperature),
		ReasoningMode:   req.ReasoningMode,
		ToolChoice:      cloneToolChoice(req.ToolChoice),
		PromptPlan:      clonePromptPlan(req.PromptPlan),
	}
}

func cloneMessages(src []llm.Message) []llm.Message {
	if len(src) == 0 {
		return nil
	}
	cloned := subagentctl.ClonePromptCacheSnapshot(&subagentctl.PromptCacheSnapshot{BaseMessages: src})
	return cloned.BaseMessages
}

func cloneToolSpecs(src []llm.ToolSpec) []llm.ToolSpec {
	if len(src) == 0 {
		return nil
	}
	cloned := subagentctl.ClonePromptCacheSnapshot(&subagentctl.PromptCacheSnapshot{Tools: src})
	return cloned.Tools
}

func nilIfNil(plan *llm.PromptPlan) *llm.PromptPlan {
	if plan == nil {
		return &llm.PromptPlan{}
	}
	return plan
}

func intPtrEqual(left *int, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func floatPtrEqual(left *float64, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
