package fileops

import (
	"context"
	"path/filepath"
	"testing"

	"arkloop/services/shared/objectstore"
)

func openFileOpsTestStore(t *testing.T, dir string) objectstore.Store {
	t.Helper()
	store, err := objectstore.NewFilesystemOpener(dir).Open(context.Background(), "tool-output-test")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

func TestLocalBackendWriteFile_UsesInternalToolOutputRoot(t *testing.T) {
	dataDir := t.TempDir()
	workDir := t.TempDir()
	scopeID := "thread-1"
	store := openFileOpsTestStore(t, dataDir)

	backend := &LocalBackend{WorkDir: workDir, ToolOutputScopeID: scopeID, ToolOutputStore: store}
	path := filepath.Join(toolOutputsVirtualRoot, scopeID, "call-1.txt")
	if err := backend.WriteFile(context.Background(), path, []byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	data, err := backend.ReadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestLocalBackendRejectsOtherToolOutputScope(t *testing.T) {
	dataDir := t.TempDir()
	workDir := t.TempDir()

	backend := &LocalBackend{WorkDir: workDir, ToolOutputScopeID: "thread-1", ToolOutputStore: openFileOpsTestStore(t, dataDir)}
	path := filepath.Join(toolOutputsVirtualRoot, "thread-2", "call-1.txt")
	if err := backend.WriteFile(context.Background(), path, []byte("nope")); err == nil {
		t.Fatal("expected cross-scope write to fail")
	}
}
