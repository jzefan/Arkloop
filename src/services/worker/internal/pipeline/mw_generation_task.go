package pipeline

import (
	"context"
	"strings"

	"arkloop/services/worker/internal/llm"
)

func NewGenerationTaskMiddleware() RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		task, _ := rc.InputJSON["generation_task"].(string)
		switch strings.ToLower(strings.TrimSpace(task)) {
		case "image":
			rc.ToolChoice = &llm.ToolChoice{Mode: "specific", ToolName: "image_generate"}
		case "video":
			rc.ToolChoice = &llm.ToolChoice{Mode: "specific", ToolName: "video_generate"}
		}
		return next(ctx, rc)
	}
}
