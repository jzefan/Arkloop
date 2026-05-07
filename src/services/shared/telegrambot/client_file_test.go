package telegrambot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientGetFile(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/botTOK/getFile") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"abc","file_path":"photos/a.jpg","file_size":4}}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	f, err := c.GetFile(context.Background(), "TOK", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if f.FilePath != "photos/a.jpg" || f.FileID != "abc" {
		t.Fatalf("got %+v", f)
	}
}

func TestClientDownloadBotFile(t *testing.T) {
	t.Parallel()
	payload := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/file/botTOK/photos/a.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	got, ct, err := c.DownloadBotFile(context.Background(), "TOK", "photos/a.jpg", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/png" {
		t.Fatalf("content-type: %q", ct)
	}
	if string(got) != string(payload) {
		t.Fatalf("bytes mismatch")
	}
}

func TestClientDownloadBotFileMaxBytes(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, strings.NewReader("abcd"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	_, _, err := c.DownloadBotFile(context.Background(), "TOK", "x.bin", 3)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected overflow error, got %v", err)
	}
}
