package fileops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"arkloop/services/shared/objectstore"
)

func TestSandboxExecDoesNotSendHostCwd(t *testing.T) {
	var seen sandboxProcessExecRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sandboxProcessExecResponse{
			Status:   "exited",
			Stdout:   "",
			Stderr:   "",
			ExitCode: intPtr(0),
		})
	}))
	defer server.Close()

	backend := &SandboxExecBackend{
		baseURL:      server.URL,
		sessionID:    "run-1/file",
		workspaceRef: "ws-test",
	}
	if _, _, _, err := backend.exec(context.Background(), "pwd", 1000); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if seen.Cwd != "" {
		t.Fatalf("expected sandbox exec cwd to be empty, got %q", seen.Cwd)
	}
}

func TestSandboxExecNormalizePathKeepsGuestRelativePath(t *testing.T) {
	backend := &SandboxExecBackend{}
	if got := backend.NormalizePath("./src/../src/app.txt"); got != "src/app.txt" {
		t.Fatalf("unexpected normalized path: %q", got)
	}
}

func TestSandboxExecToolOutputsUseHostStorage(t *testing.T) {
	dataDir := t.TempDir()
	store, err := objectstore.NewFilesystemOpener(dataDir).Open(context.Background(), "tool-output-test")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	backend := &SandboxExecBackend{toolOutputScopeID: "thread-1", toolOutputStore: store}
	path := filepath.Join(toolOutputsVirtualRoot, "thread-1", "run-1", "call-1.txt")
	if err := backend.WriteFile(context.Background(), path, []byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	info, err := backend.Stat(context.Background(), path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Size != 5 {
		t.Fatalf("unexpected size: %d", info.Size)
	}
	data, err := backend.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	if _, err := store.Head(context.Background(), filepath.ToSlash(filepath.Join("tool-outputs", "thread-1", "run-1", "call-1.txt"))); err != nil {
		t.Fatalf("expected object to exist: %v", err)
	}
}

func intPtr(v int) *int { return &v }
