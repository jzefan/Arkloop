//go:build desktop

package fileops

import (
	"strings"

	shareddesktop "arkloop/services/shared/desktop"
	sharedtoolruntime "arkloop/services/shared/toolruntime"
)

func useSandboxBackend(snapshot *sharedtoolruntime.RuntimeSnapshot) bool {
	if snapshot == nil || strings.TrimSpace(snapshot.SandboxBaseURL) == "" {
		return false
	}
	return strings.TrimSpace(shareddesktop.GetExecutionMode()) == "vm"
}
