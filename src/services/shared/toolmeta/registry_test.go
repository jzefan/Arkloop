package toolmeta

import (
	"strings"
	"testing"
)

func TestSandboxToolDescriptionsExplainWorkspaceAndArtifacts(t *testing.T) {
	python := Must("python_execute").LLMDescription
	if !strings.Contains(python, "/workspace/") || !strings.Contains(python, "/tmp/output/") {
		t.Fatalf("python_execute description should mention /workspace/ and /tmp/output/: %s", python)
	}

	execDesc := Must("exec_command").LLMDescription
	if !strings.Contains(execDesc, "/workspace/") || !strings.Contains(execDesc, "/tmp/output/") {
		t.Fatalf("exec_command description should mention /workspace/ and /tmp/output/: %s", execDesc)
	}

	stdinDesc := Must("write_stdin").LLMDescription
	if !strings.Contains(stdinDesc, "session_ref") || !strings.Contains(stdinDesc, "/workspace") {
		t.Fatalf("write_stdin description should mention session_ref and /workspace: %s", stdinDesc)
	}
}
