package objectstore

import (
	"context"
	"sort"
	"strings"
	"testing"
)

func TestFilesystemStorePutGetDelete(t *testing.T) {
	store := openFilesystemStore(t, "bucket-a")

	if err := store.Put(context.Background(), "threads/demo/file.txt", []byte("hello")); err != nil {
		t.Fatalf("put: %v", err)
	}
	data, err := store.Get(context.Background(), "threads/demo/file.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected data: %q", data)
	}
	if err := store.Delete(context.Background(), "threads/demo/file.txt"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(context.Background(), "threads/demo/file.txt"); !IsNotFound(err) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestFilesystemStorePutObjectHeadAndContentType(t *testing.T) {
	store := openFilesystemStore(t, "bucket-a")
	payload := []byte("hello metadata")

	err := store.PutObject(context.Background(), "runs/demo/output.txt", payload, PutOptions{
		ContentType: "text/plain",
		Metadata: map[string]string{
			"Owner":    "arkloop",
			" Thread ": "demo",
		},
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}

	head, err := store.Head(context.Background(), "runs/demo/output.txt")
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.ContentType != "text/plain" {
		t.Fatalf("unexpected content type: %q", head.ContentType)
	}
	if head.Size != int64(len(payload)) {
		t.Fatalf("unexpected size: %d", head.Size)
	}
	if head.Metadata["owner"] != "arkloop" || head.Metadata["thread"] != "demo" {
		t.Fatalf("unexpected metadata: %#v", head.Metadata)
	}
	if strings.TrimSpace(head.ETag) == "" {
		t.Fatal("expected etag")
	}

	data, contentType, err := store.GetWithContentType(context.Background(), "runs/demo/output.txt")
	if err != nil {
		t.Fatalf("get with content type: %v", err)
	}
	if contentType != "text/plain" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected data: %q", data)
	}
}

func TestFilesystemStorePutIfAbsentListPrefixAndWriteJSONAtomic(t *testing.T) {
	store := openFilesystemStore(t, "bucket-a")
	created, err := store.PutIfAbsent(context.Background(), "prefix/a.json", []byte("first"))
	if err != nil {
		t.Fatalf("put if absent: %v", err)
	}
	if !created {
		t.Fatal("expected first object to be created")
	}
	created, err = store.PutIfAbsent(context.Background(), "prefix/a.json", []byte("second"))
	if err != nil {
		t.Fatalf("put if absent twice: %v", err)
	}
	if created {
		t.Fatal("expected second put to be ignored")
	}
	if err := store.WriteJSONAtomic(context.Background(), "prefix/b.json", map[string]any{"revision": "rev-1"}); err != nil {
		t.Fatalf("write json atomic: %v", err)
	}

	objects, err := store.ListPrefix(context.Background(), "prefix/")
	if err != nil {
		t.Fatalf("list prefix: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("unexpected object count: %d", len(objects))
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	if objects[0].Key != "prefix/a.json" || objects[1].Key != "prefix/b.json" {
		t.Fatalf("unexpected keys: %#v", objects)
	}
	if objects[0].ETag == "" || objects[1].ETag == "" {
		t.Fatalf("expected etag for all objects: %#v", objects)
	}
}

func TestFilesystemStoreBucketIsolation(t *testing.T) {
	rootDir := t.TempDir()
	opener := NewFilesystemOpener(rootDir)
	leftRaw, err := opener.Open(context.Background(), "bucket-left")
	if err != nil {
		t.Fatalf("open left bucket: %v", err)
	}
	rightRaw, err := opener.Open(context.Background(), "bucket-right")
	if err != nil {
		t.Fatalf("open right bucket: %v", err)
	}
	left := leftRaw.(*FilesystemStore)
	right := rightRaw.(*FilesystemStore)

	if err := left.Put(context.Background(), "same/key.txt", []byte("left")); err != nil {
		t.Fatalf("put left: %v", err)
	}
	if _, err := right.Head(context.Background(), "same/key.txt"); !IsNotFound(err) {
		t.Fatalf("expected right bucket to be isolated, got %v", err)
	}
}

func TestFilesystemStoreRejectsEscapingKeys(t *testing.T) {
	store := openFilesystemStore(t, "bucket-a")
	for _, key := range []string{"../escape", "/absolute", "nested/../escape", `windows\\path`} {
		if err := store.Put(context.Background(), key, []byte("x")); err == nil {
			t.Fatalf("expected key %q to be rejected", key)
		}
	}
}

func TestFilesystemStoreSetLifecycleExpirationDaysNoop(t *testing.T) {
	store := openFilesystemStore(t, SessionStateBucket)
	if err := store.Put(context.Background(), "sessions/sh_1/restore/rev-1.json", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("put restore state: %v", err)
	}
	if err := store.SetLifecycleExpirationDays(context.Background(), 1); err != nil {
		t.Fatalf("set lifecycle: %v", err)
	}
	if _, err := store.Get(context.Background(), "sessions/sh_1/restore/rev-1.json"); err != nil {
		t.Fatalf("expected restore state to remain: %v", err)
	}
}

func openFilesystemStore(t *testing.T, bucket string) *FilesystemStore {
	t.Helper()
	opener := NewFilesystemOpener(t.TempDir())
	rawStore, err := opener.Open(context.Background(), bucket)
	if err != nil {
		t.Fatalf("open filesystem store: %v", err)
	}
	store, ok := rawStore.(*FilesystemStore)
	if !ok {
		t.Fatalf("unexpected store type: %T", rawStore)
	}
	return store
}
