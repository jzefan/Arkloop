package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"arkloop/services/shared/objectstore"
	"github.com/google/uuid"
)

func openCleanupTestStore(t *testing.T, dir string) objectstore.Store {
	t.Helper()
	store, err := objectstore.NewFilesystemOpener(dir).Open(context.Background(), "tool-output-test")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

func TestCleanupToolOutputThread_EmptyArgs(t *testing.T) {
	CleanupToolOutputThread(context.Background(), nil, "")
}

func TestCleanupToolOutputThread_NormalDeletion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", dir)
	t.Setenv("ARKLOOP_STORAGE_BACKEND", "filesystem")
	t.Setenv("ARKLOOP_STORAGE_ROOT", dir)
	threadID := uuid.MustParse("77777777-7777-7777-7777-777777777777").String()
	store := openCleanupTestStore(t, dir)
	key := filepath.ToSlash(filepath.Join("tool-outputs", threadID, "run-1", "file.txt"))
	if err := store.PutObject(context.Background(), key, []byte("data"), objectstore.PutOptions{Metadata: map[string]string{"updated_at": time.Now().UTC().Format(time.RFC3339Nano)}}); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}

	CleanupToolOutputThread(context.Background(), store, threadID)

	if _, err := store.Head(context.Background(), key); err == nil {
		t.Fatalf("expected object to be removed")
	}
}

func TestCleanupToolOutputThread_InvalidThreadID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", dir)
	t.Setenv("ARKLOOP_STORAGE_BACKEND", "filesystem")
	t.Setenv("ARKLOOP_STORAGE_ROOT", dir)
	malicious := "../etc"
	target := filepath.Join(dir, "tool-outputs", malicious)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	CleanupToolOutputThread(context.Background(), openCleanupTestStore(t, dir), malicious)

	_, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected directory to remain for invalid thread_id: %v", err)
	}
}

func TestCleanupExpiredToolOutputThreads_DeletesOldThreadDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", dir)
	t.Setenv("ARKLOOP_STORAGE_BACKEND", "filesystem")
	t.Setenv("ARKLOOP_STORAGE_ROOT", dir)
	store := openCleanupTestStore(t, dir)
	oldThread := uuid.MustParse("88888888-8888-8888-8888-888888888888").String()
	newThread := uuid.MustParse("99999999-9999-9999-9999-999999999999").String()

	oldTime := time.Now().Add(-45 * 24 * time.Hour)
	oldKey := filepath.ToSlash(filepath.Join("tool-outputs", oldThread, "run-1", "file.txt"))
	newKey := filepath.ToSlash(filepath.Join("tool-outputs", newThread, "run-2", "file.txt"))
	if err := store.PutObject(context.Background(), oldKey, []byte("old"), objectstore.PutOptions{Metadata: map[string]string{"updated_at": oldTime.UTC().Format(time.RFC3339Nano)}}); err != nil {
		t.Fatalf("setup old failed: %v", err)
	}
	if err := store.PutObject(context.Background(), newKey, []byte("new"), objectstore.PutOptions{Metadata: map[string]string{"updated_at": time.Now().UTC().Format(time.RFC3339Nano)}}); err != nil {
		t.Fatalf("setup new failed: %v", err)
	}

	deleted, err := CleanupExpiredToolOutputThreads(context.Background(), store, time.Now(), 30*24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted thread, got %d", deleted)
	}
	if _, err := store.Head(context.Background(), oldKey); err == nil {
		t.Fatalf("expected old thread removed")
	}
	if _, err := store.Head(context.Background(), newKey); err != nil {
		t.Fatalf("expected new thread to remain: %v", err)
	}
}
