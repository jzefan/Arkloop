package tools

import (
	"log/slog"
	"os"
	"path/filepath"

	"arkloop/services/worker/internal/tools/builtin/fileops"
)

// CleanupPersistedToolOutputs removes the tool-outputs/{thread_id} directory
// created by PersistLargeResult. It is safe to call when the directory does
// not exist. Callers must ensure threadID is a trusted identifier (e.g. a UUID).
func CleanupPersistedToolOutputs(threadID string) {
	if threadID == "" {
		return
	}
	if !validToolCallIDRe.MatchString(threadID) {
		slog.Warn("tool outputs cleanup skipped: invalid thread_id", "thread_id", threadID)
		return
	}
	dir := filepath.Join(fileops.ToolOutputRoot(), threadID)
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("tool outputs cleanup failed", "thread_id", threadID, "path", dir, "error", err.Error())
	}
}
