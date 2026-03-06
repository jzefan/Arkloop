package main

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	shellapi "arkloop/services/sandbox/internal/shell"

	"github.com/klauspost/compress/zstd"
)

func bindShellDirs(t *testing.T, workspace string) {
	t.Helper()
	home := filepath.Join(workspace, "home")
	temp := filepath.Join(workspace, "tmp")
	shellWorkspaceDir = workspace
	shellHomeDir = home
	shellTempDir = temp
	t.Cleanup(func() {
		shellWorkspaceDir = defaultShellCwd
		shellHomeDir = defaultShellHome
		shellTempDir = defaultShellTempDir
	})
}

func drainShellOutput(t *testing.T, controller *ShellController, resp *shellapi.AgentShellResponse, code, msg string) string {
	t.Helper()
	if code != "" {
		t.Fatalf("shell action failed: %s %s", code, msg)
	}
	output := resp.Output
	for resp.Running {
		resp, code, msg = controller.Read(shellapi.AgentShellRequest{Cursor: resp.Cursor, YieldTimeMs: 200})
		if code != "" {
			t.Fatalf("read failed: %s %s", code, msg)
		}
		output += resp.Output
	}
	return output
}

func TestShellControllerOpenDefaultsToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	bindShellDirs(t, workspace)
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
	bindShellDirs(t, workspace)
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
	bindShellDirs(t, workspace)
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
	bindShellDirs(t, workspace)
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

func TestShellControllerCheckpointRestorePreservesState(t *testing.T) {
	workspace := t.TempDir()
	bindShellDirs(t, workspace)
	controller := NewShellController()
	if _, code, msg := controller.Open(shellapi.AgentShellRequest{}); code != "" {
		t.Fatalf("open failed: %s %s", code, msg)
	}
	command := "mkdir -p demo && cd demo && export FOO=bar && touch a.txt && pwd"
	resp, code, msg := controller.Exec(shellapi.AgentShellRequest{Command: command, YieldTimeMs: 1000, TimeoutMs: 5000})
	_ = drainShellOutput(t, controller, resp, code, msg)
	checkpoint, code, msg := controller.CheckpointExport()
	if code != "" || msg != "" {
		t.Fatalf("checkpoint failed: %s %s", code, msg)
	}
	if checkpoint.Cwd != filepath.Join(workspace, "demo") {
		t.Fatalf("unexpected checkpoint cwd: %s", checkpoint.Cwd)
	}
	if _, code, msg := controller.Close(); code != "" {
		t.Fatalf("close failed: %s %s", code, msg)
	}

	restored := NewShellController()
	if _, code, msg := restored.RestoreImport(shellapi.AgentCheckpointRequest{Archive: checkpoint.Archive}); code != "" || msg != "" {
		t.Fatalf("restore import failed: %s %s", code, msg)
	}
	if _, code, msg := restored.Open(shellapi.AgentShellRequest{Cwd: checkpoint.Cwd, Env: checkpoint.Env}); code != "" {
		t.Fatalf("restored open failed: %s %s", code, msg)
	}
	verify, code, msg := restored.Exec(shellapi.AgentShellRequest{Command: "printf '%s\n' \"$FOO\" && pwd && test -f a.txt && echo ok", YieldTimeMs: 1000, TimeoutMs: 5000})
	verifyOutput := drainShellOutput(t, restored, verify, code, msg)
	if !strings.Contains(verifyOutput, "bar") || !strings.Contains(verifyOutput, filepath.Join(workspace, "demo")) || !strings.Contains(verifyOutput, "ok") {
		t.Fatalf("unexpected restored output: %q", verifyOutput)
	}
	_, _, _ = restored.Close()
	_ = resp
}

func TestShellControllerOpenWithEnvKeepsFixedHome(t *testing.T) {
	workspace := t.TempDir()
	bindShellDirs(t, workspace)
	controller := NewShellController()
	if _, code, msg := controller.Open(shellapi.AgentShellRequest{Env: map[string]string{"HOME": "/tmp/evil", "FOO": "bar"}}); code != "" {
		t.Fatalf("open failed: %s %s", code, msg)
	}
	resp, code, msg := controller.Exec(shellapi.AgentShellRequest{Command: "printf '%s|%s' \"$HOME\" \"$FOO\"", YieldTimeMs: 1000, TimeoutMs: 5000})
	output := drainShellOutput(t, controller, resp, code, msg)
	if !strings.Contains(output, shellHomeDir+"|bar") {
		t.Fatalf("unexpected env output: %q", output)
	}
	_, _, _ = controller.Close()
}

func TestShellControllerRestoreRejectsEscapingSymlink(t *testing.T) {
	workspace := t.TempDir()
	bindShellDirs(t, workspace)
	controller := NewShellController()
	archive := base64.StdEncoding.EncodeToString(mustCheckpointArchive(t, func(tw *tar.Writer) {
		writeTarHeader(t, tw, &tar.Header{Name: "workspace/", Typeflag: tar.TypeDir, Mode: 0o755})
		writeTarHeader(t, tw, &tar.Header{Name: "workspace/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777})
	}))
	_, code, msg := controller.RestoreImport(shellapi.AgentCheckpointRequest{Archive: archive})
	if code != "" {
		t.Fatalf("unexpected shell code: %s", code)
	}
	if !strings.Contains(msg, "escapes root") {
		t.Fatalf("unexpected restore error: %s", msg)
	}
}

func mustCheckpointArchive(t *testing.T, fill func(tw *tar.Writer)) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zw, err := zstd.NewWriter(&buffer)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	tw := tar.NewWriter(zw)
	fill(tw)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zstd: %v", err)
	}
	return buffer.Bytes()
}

func writeTarHeader(t *testing.T, tw *tar.Writer, header *tar.Header) {
	t.Helper()
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
		if _, err := io.WriteString(tw, "payload"); err != nil {
			t.Fatalf("write tar payload: %v", err)
		}
	}
}
