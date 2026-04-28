package llm

import (
	"context"
	"log/slog"
)

type providerCompletionDebug struct {
	ProviderKind      string
	APIMode           string
	LlmCallID         string
	AssistantMessage  *Message
	ContentPartCount  int
	ThinkingPartCount int
	VisibleTextLen    int
	ToolCallCount     int
}

func logProviderCompletionDebug(ctx context.Context, info providerCompletionDebug) {
	if ctx == nil {
		ctx = context.Background()
	}
	contentPartCount := info.ContentPartCount
	thinkingPartCount := info.ThinkingPartCount
	visibleTextLen := info.VisibleTextLen
	toolCallCount := info.ToolCallCount
	if info.AssistantMessage != nil {
		contentPartCount = len(info.AssistantMessage.Content)
		thinkingPartCount = providerThinkingPartCount(info.AssistantMessage.Content)
		visibleTextLen = len(VisibleMessageText(*info.AssistantMessage))
		if toolCallCount == 0 {
			toolCallCount = len(info.AssistantMessage.ToolCalls)
		}
	}
	slog.DebugContext(ctx, "provider_stream_completed_debug",
		"provider_kind", info.ProviderKind,
		"api_mode", info.APIMode,
		"llm_call_id", info.LlmCallID,
		"has_assistant_message", info.AssistantMessage != nil,
		"content_part_count", contentPartCount,
		"thinking_part_count", thinkingPartCount,
		"visible_text_len", visibleTextLen,
		"tool_call_count", toolCallCount,
	)
}

func providerThinkingPartCount(parts []ContentPart) int {
	count := 0
	for _, part := range parts {
		switch part.Kind() {
		case "thinking", "redacted_thinking":
			count++
		}
	}
	return count
}
