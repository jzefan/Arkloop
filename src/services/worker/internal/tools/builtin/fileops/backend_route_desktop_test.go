//go:build desktop

package fileops

import (
	"testing"

	shareddesktop "arkloop/services/shared/desktop"
	sharedtoolruntime "arkloop/services/shared/toolruntime"
)

func TestResolveBackendUsesExecutionModeOnDesktop(t *testing.T) {
	snapshot := &sharedtoolruntime.RuntimeSnapshot{
		SandboxBaseURL:   "http://sandbox.internal",
		SandboxAuthToken: "token",
	}

	shareddesktop.SetExecutionMode("local")
	if _, ok := ResolveBackend(snapshot, "/workspace", "run-1", "", "", "").(*LocalBackend); !ok {
		t.Fatal("expected local backend when desktop execution mode is local")
	}

	shareddesktop.SetExecutionMode("vm")
	if _, ok := ResolveBackend(snapshot, "/workspace", "run-1", "", "", "").(*SandboxExecBackend); !ok {
		t.Fatal("expected sandbox backend when desktop execution mode is vm")
	}
}
