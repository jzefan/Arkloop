package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"arkloop/services/worker/internal/tools/builtin/fileops"
)

const PersistThreshold = 16 * 1024 // 16KB

// PersistLargeResult writes tool outputs larger than PersistThreshold to disk
// and replaces ResultJSON with a lightweight preview containing the file path.
func PersistLargeResult(
	ctx context.Context,
	execCtx ExecutionContext,
	toolCallID string,
	toolName string,
	result ExecutionResult,
) ExecutionResult {
	_ = toolName
	if result.ResultJSON == nil {
		return result
	}
	raw, err := json.Marshal(result.ResultJSON)
	if err != nil || len(raw) <= PersistThreshold {
		return result
	}

	accountID := ""
	if execCtx.AccountID != nil {
		accountID = execCtx.AccountID.String()
	}

	backend := fileops.ResolveBackend(
		execCtx.RuntimeSnapshot,
		execCtx.WorkDir,
		execCtx.RunID.String(),
		accountID,
		execCtx.ProfileRef,
		execCtx.WorkspaceRef,
	)

	filePath := fmt.Sprintf(".tool-outputs/%s.txt", strings.TrimSpace(toolCallID))
	if writeErr := backend.WriteFile(ctx, filePath, raw); writeErr != nil {
		// Fallback: return original result and let CompressResult handle it.
		return result
	}

	preview := generatePreview(raw, 2*1024)
	originalBytes := len(raw)

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
	newResult["hint"] = "Full output saved. Use read_file to read specific sections with offset/limit."

	return ExecutionResult{
		ResultJSON:   newResult,
		ContentParts: result.ContentParts,
		Error:        result.Error,
		DurationMs:   result.DurationMs,
		Usage:        result.Usage,
		Events:       result.Events,
	}
}

func generatePreview(raw []byte, budget int) string {
	if len(raw) <= budget {
		return string(raw)
	}
	cut := make([]byte, budget)
	copy(cut, raw[:budget])
	if idx := bytes.LastIndexByte(cut, '\n'); idx > budget/2 {
		cut = cut[:idx]
	}
	return string(cut) + "\n...[truncated]"
}
