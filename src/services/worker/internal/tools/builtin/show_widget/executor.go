package showwidget

import (
	"context"
	"strings"
	"time"

	"arkloop/services/worker/internal/tools"
)

type ToolExecutor struct{}

func NewToolExecutor() *ToolExecutor { return &ToolExecutor{} }

func (e *ToolExecutor) Execute(
	_ context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()

	seenReadMe, _ := args["i_have_seen_read_me"].(bool)
	if !seenReadMe {
		return errResult("tool.args_invalid", "i_have_seen_read_me must be true after visualize_read_me", started)
	}
	if !execCtx.GenerativeUIReadMeSeen {
		return errResult("tool.execution_failed", "visualize_read_me or artifact_guidelines must be called earlier in this run", started)
	}

	widgetCode, _ := args["widget_code"].(string)
	if widgetCode == "" {
		return errResult("tool.args_invalid", "widget_code is required", started)
	}

	if err := validateLoadingMessages(args["loading_messages"]); err != "" {
		return errResult("tool.args_invalid", err, started)
	}

	title, _ := args["title"].(string)

	return tools.ExecutionResult{
		ResultJSON: map[string]any{"ok": true, "title": title},
		DurationMs: durationMs(started),
	}
}

func errResult(class, msg string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error:      &tools.ExecutionError{ErrorClass: class, Message: msg},
		DurationMs: durationMs(started),
	}
}

func validateLoadingMessages(raw any) string {
	if raw == nil {
		return ""
	}
	arr, ok := raw.([]any)
	if !ok {
		return "loading_messages must be an array"
	}
	n := len(arr)
	if n < 1 || n > 4 {
		return "loading_messages must have 1 to 4 items"
	}
	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return "loading_messages items must be non-empty strings"
		}
	}
	return ""
}

func durationMs(started time.Time) int {
	ms := int(time.Since(started) / time.Millisecond)
	if ms < 0 {
		return 0
	}
	return ms
}
