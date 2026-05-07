package tools

import (
	"strings"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/llm"
)

type referenceImageMessageProvider interface {
	ReadToolMessages() []llm.Message
}

// ReferenceImagesFromPipelineRC returns image attachments from the latest user
// message that has materialized image data. Generation tools use this so a
// user-uploaded reference image is not lost when the model omits input_images.
func ReferenceImagesFromPipelineRC(pipelineRC any, limit int) []llm.ContentPart {
	if limit <= 0 {
		return nil
	}
	provider, ok := pipelineRC.(referenceImageMessageProvider)
	if !ok || provider == nil {
		return nil
	}
	messages := provider.ReadToolMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			continue
		}
		parts := make([]llm.ContentPart, 0, limit)
		for _, part := range messages[i].Content {
			if part.Kind() != messagecontent.PartTypeImage || part.Attachment == nil || len(part.Data) == 0 {
				continue
			}
			copied := part
			copied.Data = append([]byte(nil), part.Data...)
			parts = append(parts, copied)
			if len(parts) >= limit {
				return parts
			}
		}
		if len(parts) > 0 {
			return parts
		}
	}
	return nil
}
