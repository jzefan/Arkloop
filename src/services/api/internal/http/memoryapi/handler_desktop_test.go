//go:build desktop

package memoryapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildNowledgeSnapshotBlock(t *testing.T) {
	block := buildNowledgeSnapshotBlock([]nowledgeListedMemory{
		{ID: "mem-1", Title: "偏好", Content: "用户偏好中文回复，并且希望答案短一点。"},
	})
	if !strings.Contains(block, "<memory>") {
		t.Fatalf("expected memory block wrapper, got %q", block)
	}
	if !strings.Contains(block, "[偏好] 用户偏好中文回复") {
		t.Fatalf("expected linear fragment line, got %q", block)
	}
}

func TestBuildNowledgeSnapshotHits(t *testing.T) {
	hits := buildNowledgeSnapshotHits([]nowledgeListedMemory{
		{ID: "mem-9", Title: "", Content: "这是一段用于摘要回退的内容。"},
	})
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %#v", hits)
	}
	if hits[0].URI != "nowledge://memory/mem-9" {
		t.Fatalf("unexpected uri: %#v", hits[0])
	}
	if hits[0].Abstract == "" {
		t.Fatalf("expected non-empty abstract: %#v", hits[0])
	}
	if !hits[0].IsLeaf {
		t.Fatalf("expected leaf hit: %#v", hits[0])
	}
}

func TestResolveNowledgeConfigFallsBackToLocalDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nowledge-mem")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"apiUrl":"http://127.0.0.1:14242","apiKey":"local-key"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	h := &handler{memoryProvider: "nowledge"}
	cfg, err := h.resolveNowledgeConfig()
	if err != nil {
		t.Fatalf("resolveNowledgeConfig: %v", err)
	}
	if cfg.baseURL != "http://127.0.0.1:14242" {
		t.Fatalf("unexpected base url: %#v", cfg)
	}
	if cfg.apiKey != "local-key" {
		t.Fatalf("unexpected api key: %#v", cfg)
	}
}

func TestNowledgeMemoryIDFromURI(t *testing.T) {
	id, err := nowledgeMemoryIDFromURI("nowledge://memory/mem-42")
	if err != nil {
		t.Fatalf("nowledgeMemoryIDFromURI: %v", err)
	}
	if id != "mem-42" {
		t.Fatalf("unexpected id: %q", id)
	}
}
