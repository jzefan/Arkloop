package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLimitedBuffer_Truncates(t *testing.T) {
	buf := newLimitedBuffer(10)
	n, err := buf.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != len("hello world") {
		t.Fatalf("expected n=%d, got %d", len("hello world"), n)
	}
	if got := buf.String(); got != "hello worl" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestExecuteJob_CodeTooLarge(t *testing.T) {
	job := ExecJob{
		Language:  "python",
		Code:      strings.Repeat("a", maxCodeBytes+1),
		TimeoutMs: 1000,
	}
	result := executeJob(job)
	if result.ExitCode != 1 {
		t.Fatalf("expected ExitCode=1, got %d", result.ExitCode)
	}
	if strings.TrimSpace(result.Stderr) == "" {
		t.Fatalf("expected stderr not empty")
	}
}

func TestFetchArtifactsFromDir_UsesContentType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preview.png")
	content := []byte("<!doctype html><html><body>preview</body></html>")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	result := fetchArtifactsFromDir(dir)
	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(result.Artifacts))
	}
	if got := result.Artifacts[0].MimeType; got != "text/html" {
		t.Fatalf("mime type = %q, want text/html", got)
	}
}

func TestDetectMimeType_SVG(t *testing.T) {
	data := []byte("<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	if got := detectMimeType(data); got != "image/svg+xml" {
		t.Fatalf("mime type = %q, want image/svg+xml", got)
	}
}

func TestDetectMimeType_FallbackToOctetStream(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "unknown_binary", data: []byte{0x00, 0x9f, 0x92, 0x96}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectMimeType(tc.data); got != "application/octet-stream" {
				t.Fatalf("mime type = %q, want application/octet-stream", got)
			}
		})
	}
}
