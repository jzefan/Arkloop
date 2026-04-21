package fileops

import (
	"os"
	"path/filepath"
	"strings"
)

// ToolOutputRoot returns the root directory for persisted tool outputs.
// Priority: ARKLOOP_TOOL_OUTPUT_DIR env var > ~/.arkloop/tool-outputs > /tmp/arkloop-tool-outputs
func ToolOutputRoot() string {
	if v := strings.TrimSpace(os.Getenv("ARKLOOP_TOOL_OUTPUT_DIR")); v != "" {
		if isValidToolOutputRoot(v) {
			return v
		}
		// invalid env var value: fall through to defaults
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".arkloop", "tool-outputs")
	}
	return filepath.Join(os.TempDir(), "arkloop-tool-outputs")
}

func isValidToolOutputRoot(path string) bool {
	if path == "" || path == "/" {
		return false
	}
	cleaned := filepath.Clean(path)
	if cleaned == "/" {
		return false
	}
	if strings.Contains(cleaned, "..") {
		return false
	}
	return true
}
