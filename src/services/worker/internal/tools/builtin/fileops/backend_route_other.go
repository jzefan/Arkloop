//go:build !desktop

package fileops

import (
	"strings"

	sharedtoolruntime "arkloop/services/shared/toolruntime"
)

func useSandboxBackend(snapshot *sharedtoolruntime.RuntimeSnapshot) bool {
	return snapshot != nil && strings.TrimSpace(snapshot.SandboxBaseURL) != ""
}
