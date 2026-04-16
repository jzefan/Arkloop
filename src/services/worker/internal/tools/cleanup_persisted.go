package tools

import (
	"log/slog"
	"os"
	"path/filepath"
)

// CleanupPersistedToolOutputs removes the .tool-outputs/{run_id} directory
// created by PersistLargeResult. It is safe to call when the directory does
// not exist.
func CleanupPersistedToolOutputs(workDir, runID string) {
	if workDir == "" || runID == "" {
		return
	}
	dir := filepath.Join(workDir, ".tool-outputs", runID)
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("tool outputs cleanup failed", "run_id", runID, "path", dir, "error", err.Error())
	}
}
