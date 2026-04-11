package nowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDesktopConfigPrefersExplicitValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".nowledge-mem")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"apiUrl":"http://127.0.0.1:14242","apiKey":"local-key"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved := ResolveDesktopConfig(Config{
		BaseURL:          "http://explicit.local:24242",
		APIKey:           "explicit-key",
		RequestTimeoutMs: 1234,
	})

	if resolved.BaseURL != "http://explicit.local:24242" {
		t.Fatalf("expected explicit base url, got %q", resolved.BaseURL)
	}
	if resolved.APIKey != "explicit-key" {
		t.Fatalf("expected explicit api key, got %q", resolved.APIKey)
	}
	if resolved.RequestTimeoutMs != 1234 {
		t.Fatalf("expected timeout preserved, got %d", resolved.RequestTimeoutMs)
	}
}

func TestResolveDesktopConfigFallsBackToLocalConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".nowledge-mem")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"apiUrl":"http://127.0.0.1:14242","apiKey":"local-key"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	resolved := ResolveDesktopConfig(Config{})
	if resolved.BaseURL != "http://127.0.0.1:14242" {
		t.Fatalf("unexpected base url: %q", resolved.BaseURL)
	}
	if resolved.APIKey != "local-key" {
		t.Fatalf("unexpected api key: %q", resolved.APIKey)
	}
}

func TestResolveDesktopConfigFallsBackToDefaultLoopback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	resolved := ResolveDesktopConfig(Config{})
	if resolved.BaseURL != defaultLocalBaseURL {
		t.Fatalf("unexpected base url: %q", resolved.BaseURL)
	}
	if resolved.APIKey != "" {
		t.Fatalf("expected empty api key for default loopback, got %q", resolved.APIKey)
	}
}
