package main

import (
	"strings"
	"testing"
	"time"

	shellapi "arkloop/services/sandbox/internal/shell"
)

func TestShellControllerOpenDefaultsToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	shellWorkspaceDir = workspace
	t.Cleanup(func() { shellWorkspaceDir = defaultShellCwd })
	controller := NewShellController()
	resp, code, msg := controller.Open(shellapi.AgentShellRequest{})
	if code != "" || msg != "" {
		t.Fatalf("open failed: %s %s", code, msg)
	}
	if resp.Cwd != workspace {
		t.Fatalf("expected cwd %s, got %s", workspace, resp.Cwd)
	}
	_, _, _ = controller.Close()
}

func TestShellControllerExecPreservesCwd(t *testing.T) {
	workspace := t.TempDir()
	shellWorkspaceDir = workspace
	t.Cleanup(func() { shellWorkspaceDir = defaultShellCwd })
	controller := NewShellController()
	if _, code, msg := controller.Open(shellapi.AgentShellRequest{}); code != "" {
		t.Fatalf("open failed: %s %s", code, msg)
	}

	resp, code, msg := controller.Exec(shellapi.AgentShellRequest{Command: "cd /tmp && pwd", YieldTimeMs: 1000, TimeoutMs: 5000})
	if code != "" {
		t.Fatalf("exec failed: %s %s", code, msg)
	}
	if resp.Cwd != "/tmp" {
		t.Fatalf("expected cwd /tmp, got %s", resp.Cwd)
	}

	resp, code, msg = controller.Exec(shellapi.AgentShellRequest{Command: "pwd", YieldTimeMs: 1000, TimeoutMs: 5000})
	if code != "" {
		t.Fatalf("exec failed: %s %s", code, msg)
	}
	if !strings.Contains(resp.Output, "/tmp") {
		t.Fatalf("expected output to contain /tmp, got %q", resp.Output)
	}
	_, _, _ = controller.Close()
}

func TestShellControllerReadTruncatedWindow(t *testing.T) {
	workspace := t.TempDir()
	shellWorkspaceDir = workspace
	t.Cleanup(func() { shellWorkspaceDir = defaultShellCwd })
	controller := NewShellController()
	if _, code, msg := controller.Open(shellapi.AgentShellRequest{}); code != "" {
		t.Fatalf("open failed: %s %s", code, msg)
	}

	resp, code, msg := controller.Exec(shellapi.AgentShellRequest{Command: "python3 - <<'PY'\nprint('x'*2000000)\nPY", YieldTimeMs: 1000, TimeoutMs: 5000})
	if code != "" {
		t.Fatalf("exec failed: %s %s", code, msg)
	}
	for resp.Running {
		resp, code, msg = controller.Read(shellapi.AgentShellRequest{Cursor: resp.Cursor, YieldTimeMs: 200})
		if code != "" {
			t.Fatalf("read failed: %s %s", code, msg)
		}
	}
	read, code, msg := controller.Read(shellapi.AgentShellRequest{Cursor: 0, YieldTimeMs: 10})
	if code != "" {
		t.Fatalf("read failed: %s %s", code, msg)
	}
	if !read.Truncated {
		t.Fatal("expected truncated read")
	}
	_, _, _ = controller.Close()
}

func TestShellControllerWriteAndSignal(t *testing.T) {
	workspace := t.TempDir()
	shellWorkspaceDir = workspace
	t.Cleanup(func() { shellWorkspaceDir = defaultShellCwd })
	controller := NewShellController()
	if _, code, msg := controller.Open(shellapi.AgentShellRequest{}); code != "" {
		t.Fatalf("open failed: %s %s", code, msg)
	}

	resp, code, msg := controller.Exec(shellapi.AgentShellRequest{Command: "python3 -c \"name=input(); print(name)\"", YieldTimeMs: 200, TimeoutMs: 5000})
	if code != "" {
		t.Fatalf("exec failed: %s %s", code, msg)
	}
	if !resp.Running {
		t.Fatal("expected interactive command to keep running")
	}

	resp, code, msg = controller.Write(shellapi.AgentShellRequest{Input: "arkloop\n", YieldTimeMs: 1000})
	if code != "" {
		t.Fatalf("write failed: %s %s", code, msg)
	}
	if !strings.Contains(resp.Output, "arkloop") {
		t.Fatalf("expected echoed output, got %q", resp.Output)
	}

	resp, code, msg = controller.Exec(shellapi.AgentShellRequest{Command: "sleep 10", YieldTimeMs: 100, TimeoutMs: 5000})
	if code != "" {
		t.Fatalf("exec failed: %s %s", code, msg)
	}
	if !resp.Running {
		t.Fatal("expected sleep to keep running")
	}
	time.Sleep(200 * time.Millisecond)
	resp, code, msg = controller.Signal(shellapi.AgentShellRequest{Signal: shellapi.SignalINT, YieldTimeMs: 1000})
	if code != "" {
		t.Fatalf("signal failed: %s %s", code, msg)
	}
	if resp.Running {
		t.Fatal("expected session to stop running after signal")
	}
	_, _, _ = controller.Close()
}
