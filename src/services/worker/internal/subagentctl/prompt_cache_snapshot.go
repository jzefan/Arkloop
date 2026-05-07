package subagentctl

import (
	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/llm"
)

func ClonePromptCacheSnapshot(src *PromptCacheSnapshot) *PromptCacheSnapshot {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.BaseMessages = cloneLLMMessages(src.BaseMessages)
	cloned.Messages = cloneLLMMessages(src.Messages)
	cloned.Tools = cloneToolSpecs(src.Tools)
	cloned.MaxOutputTokens = cloneIntPtr(src.MaxOutputTokens)
	cloned.Temperature = cloneFloatPtr(src.Temperature)
	cloned.ToolChoice = cloneToolChoice(src.ToolChoice)
	cloned.PromptPlan = clonePromptPlan(src.PromptPlan)
	return &cloned
}

func cloneLLMMessages(src []llm.Message) []llm.Message {
	if len(src) == 0 {
		return nil
	}
	out := make([]llm.Message, len(src))
	for i, msg := range src {
		out[i] = msg
		if len(msg.Content) > 0 {
			out[i].Content = make([]llm.ContentPart, len(msg.Content))
			for j, part := range msg.Content {
				out[i].Content[j] = part
				if part.Attachment != nil {
					attachment := *part.Attachment
					out[i].Content[j].Attachment = &attachment
				}
				if part.CacheHint != nil {
					hint := *part.CacheHint
					out[i].Content[j].CacheHint = &hint
				}
				if part.CacheControl != nil {
					cacheControl := *part.CacheControl
					out[i].Content[j].CacheControl = &cacheControl
				}
				if len(part.Data) > 0 {
					out[i].Content[j].Data = append([]byte(nil), part.Data...)
				}
			}
		}
		if len(msg.ToolCalls) > 0 {
			out[i].ToolCalls = make([]llm.ToolCall, len(msg.ToolCalls))
			for j, call := range msg.ToolCalls {
				out[i].ToolCalls[j] = llm.ToolCall{
					ToolCallID:         call.ToolCallID,
					ToolName:           call.ToolName,
					ArgumentsJSON:      cloneSnapshotMap(call.ArgumentsJSON),
					DisplayDescription: call.DisplayDescription,
				}
			}
		}
		if msg.Phase != nil {
			phase := *msg.Phase
			out[i].Phase = &phase
		}
		if msg.OutputTokens != nil {
			tokens := *msg.OutputTokens
			out[i].OutputTokens = &tokens
		}
	}
	return out
}

func cloneToolSpecs(src []llm.ToolSpec) []llm.ToolSpec {
	if len(src) == 0 {
		return nil
	}
	out := make([]llm.ToolSpec, len(src))
	for i, spec := range src {
		out[i] = spec
		out[i].JSONSchema = cloneSnapshotMap(spec.JSONSchema)
		if spec.Description != nil {
			description := *spec.Description
			out[i].Description = &description
		}
		if spec.CacheHint != nil {
			hint := *spec.CacheHint
			out[i].CacheHint = &hint
		}
	}
	return out
}

func clonePromptPlan(src *llm.PromptPlan) *llm.PromptPlan {
	if src == nil {
		return nil
	}
	cloned := *src
	if len(src.SystemBlocks) > 0 {
		cloned.SystemBlocks = append([]llm.PromptPlanBlock(nil), src.SystemBlocks...)
	}
	if len(src.MessageBlocks) > 0 {
		cloned.MessageBlocks = append([]llm.PromptPlanBlock(nil), src.MessageBlocks...)
	}
	if len(src.MessageCache.PinnedCacheEdits) > 0 {
		cloned.MessageCache.PinnedCacheEdits = append([]llm.PromptCacheEditsBlock(nil), src.MessageCache.PinnedCacheEdits...)
		for i, block := range cloned.MessageCache.PinnedCacheEdits {
			if len(block.Edits) > 0 {
				cloned.MessageCache.PinnedCacheEdits[i].Edits = append([]llm.PromptCacheEdit(nil), block.Edits...)
			}
		}
	}
	if src.MessageCache.NewCacheEdits != nil {
		block := *src.MessageCache.NewCacheEdits
		if len(block.Edits) > 0 {
			block.Edits = append([]llm.PromptCacheEdit(nil), block.Edits...)
		}
		cloned.MessageCache.NewCacheEdits = &block
	}
	return &cloned
}

func cloneToolChoice(src *llm.ToolChoice) *llm.ToolChoice {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneIntPtr(src *int) *int {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func cloneFloatPtr(src *float64) *float64 {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func cloneSnapshotMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneSnapshotAny(value)
	}
	return out
}

func cloneSnapshotAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSnapshotMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneSnapshotAny(item)
		}
		return out
	case []string:
		if typed == nil {
			return []string(nil)
		}
		return append(make([]string, 0, len(typed)), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case messagecontent.AttachmentRef:
		return typed
	case *messagecontent.AttachmentRef:
		if typed == nil {
			return (*messagecontent.AttachmentRef)(nil)
		}
		cloned := *typed
		return &cloned
	default:
		return value
	}
}
