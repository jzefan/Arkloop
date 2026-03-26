package executor

import (
	"fmt"
	"strings"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/routing"
)

func filterImagePartsForRoute(selected *routing.SelectedProviderRoute, parts []llm.ContentPart) []llm.ContentPart {
	if supportsImageInput(selected) {
		return parts
	}
	out := make([]llm.ContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Kind() == messagecontent.PartTypeImage {
			out = append(out, llm.ContentPart{Type: messagecontent.PartTypeText, Text: imagePlaceholder(part)})
			continue
		}
		out = append(out, part)
	}
	return out
}

func supportsImageInput(selected *routing.SelectedProviderRoute) bool {
	if selected == nil {
		return false
	}
	caps := routing.SelectedRouteModelCapabilities(selected)
	return caps.SupportsInputModality("image")
}

func imagePlaceholder(part llm.ContentPart) string {
	if part.Attachment != nil {
		if name := strings.TrimSpace(part.Attachment.Filename); name != "" {
			return fmt.Sprintf("[图片: %s] 当前模型不能直接查看图片；如需理解图片内容，请调用 understand_image 工具。", name)
		}
	}
	return "[图片] 当前模型不能直接查看图片；如需理解图片内容，请调用 understand_image 工具。"
}

func applyImageFilter(route *routing.SelectedProviderRoute, messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return messages
	}
	out := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		parts := filterImagePartsForRoute(route, msg.Content)
		out = append(out, llm.Message{
			Role:         msg.Role,
			Content:      parts,
			ToolCalls:    msg.ToolCalls,
			OutputTokens: msg.OutputTokens,
		})
	}
	return out
}
