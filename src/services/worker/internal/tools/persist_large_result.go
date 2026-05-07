package tools

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"arkloop/services/worker/internal/tools/builtin/fileops"
)

const (
	PersistThreshold   = 50 * 1024 // 50KB
	PersistPreviewHead = 4 * 1024  // 4KB head
	PersistPreviewTail = 4 * 1024  // 4KB tail
)

var validToolCallIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// PersistLargeResult writes tool outputs larger than PersistThreshold to disk
// and replaces ResultJSON with a lightweight preview containing the file path.
// The raw bytes must be the JSON-marshaled form of result.ResultJSON.
func PersistLargeResult(
	ctx context.Context,
	execCtx ExecutionContext,
	toolCallID string,
	raw []byte,
	result ExecutionResult,
) ExecutionResult {
	if result.ResultJSON == nil || len(raw) <= PersistThreshold {
		return result
	}
	toolCallID = strings.TrimSpace(toolCallID)
	if !validToolCallIDRe.MatchString(toolCallID) {
		slog.Warn("persist_large_result: invalid tool_call_id, skipping persistence", "tool_call_id", toolCallID)
		return result
	}

	// extract raw output text; try "output" then "stdout", fall back to full JSON
	content := raw
	for _, key := range []string{"output", "stdout"} {
		if out, ok := result.ResultJSON[key]; ok {
			if v, ok := out.(string); ok {
				content = []byte(v)
				break
			}
		}
	}

	threadID := ""
	if execCtx.ThreadID != nil {
		threadID = execCtx.ThreadID.String()
	} else {
		threadID = execCtx.RunID.String()
	}
	filePath := filepath.Join(fileops.ToolOutputRoot(), threadID, toolCallID+".txt")

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		slog.Warn("persist_large_result: mkdir failed, falling back to compression",
			"run_id", execCtx.RunID.String(),
			"tool_call_id", toolCallID,
			"filepath", filePath,
			"error", err.Error(),
		)
		return result
	}
	if writeErr := os.WriteFile(filePath, content, 0o644); writeErr != nil {
		slog.Warn("persist_large_result: write failed, falling back to compression",
			"run_id", execCtx.RunID.String(),
			"tool_call_id", toolCallID,
			"filepath", filePath,
			"error", writeErr.Error(),
		)
		return result
	}

	preview := generatePreview(content, PersistPreviewHead, PersistPreviewTail)
	originalBytes := len(content)

	newResult := make(map[string]any, len(metadataFields)+5)
	for k, v := range result.ResultJSON {
		if _, keep := metadataFields[k]; keep {
			newResult[k] = v
		}
	}
	newResult["persisted"] = true
	newResult["filepath"] = filePath
	newResult["original_bytes"] = originalBytes
	newResult["preview"] = preview
	newResult["hint"] = "Full output saved. Use grep to search the file, or read with offset/limit to page through it. Do NOT read the entire file."

	return ExecutionResult{
		ResultJSON:   newResult,
		ContentParts: result.ContentParts,
		Error:        result.Error,
		DurationMs:   result.DurationMs,
		Usage:        result.Usage,
		Events:       result.Events,
	}
}

func generatePreview(raw []byte, headBudget, tailBudget int) string {
	totalBudget := headBudget + tailBudget
	if len(raw) <= totalBudget {
		return string(raw)
	}

	// head: first headBudget bytes, cut at newline boundary
	head := make([]byte, headBudget)
	copy(head, raw[:headBudget])
	if idx := bytes.LastIndexByte(head, '\n'); idx > headBudget/2 {
		head = head[:idx]
	}
	// ensure valid UTF-8: drop incomplete trailing rune
	for len(head) > 0 && !utf8.Valid(head) {
		_, size := utf8.DecodeLastRune(head)
		head = head[:len(head)-size]
	}

	// tail: last tailBudget bytes, cut at newline boundary
	tailStart := len(raw) - tailBudget
	tail := raw[tailStart:]
	if idx := bytes.IndexByte(tail, '\n'); idx >= 0 && idx < len(tail)/2 {
		tail = tail[idx+1:]
	}
	// ensure valid UTF-8: drop incomplete leading rune
	for len(tail) > 0 && !utf8.Valid(tail) {
		_, size := utf8.DecodeRune(tail)
		tail = tail[size:]
	}

	truncated := len(raw) - len(head) - len(tail)
	return string(head) + fmt.Sprintf("\n...[%d bytes truncated]...\n", truncated) + string(tail)
}
