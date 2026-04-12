package executor

import (
	"strings"

	"arkloop/services/worker/internal/llm"
)

func appendRuntimePromptMessage(messages []llm.Message, runtimePrompt string) []llm.Message {
	cleaned := strings.TrimSpace(runtimePrompt)
	if cleaned == "" {
		return messages
	}
	return append(messages, llm.Message{
		Role:    "user",
		Content: []llm.TextPart{{Text: cleaned}},
	})
}
