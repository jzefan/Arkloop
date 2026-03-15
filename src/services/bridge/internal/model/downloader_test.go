package model

import (
	"os"
	"path/filepath"
	"testing"

	"arkloop/services/bridge/internal/docker"
)

type testLogger struct{}

func (testLogger) Info(string, map[string]any)  {}
func (testLogger) Error(string, map[string]any) {}

func TestInstalledVariantReturnsKnownVariantOnly(t *testing.T) {
	dir := t.TempDir()
	dl := NewDownloader(dir, testLogger{})

	if got := dl.InstalledVariant(); got != "" {
		t.Fatalf("InstalledVariant() = %q, want empty", got)
	}

	if err := osWriteFile(filepath.Join(dir, installedVariantFilename), []byte("22m\n")); err != nil {
		t.Fatalf("write variant metadata: %v", err)
	}
	if got := dl.InstalledVariant(); got != "22m" {
		t.Fatalf("InstalledVariant() = %q, want 22m", got)
	}

	if err := osWriteFile(filepath.Join(dir, installedVariantFilename), []byte("unknown\n")); err != nil {
		t.Fatalf("overwrite variant metadata: %v", err)
	}
	if got := dl.InstalledVariant(); got != "" {
		t.Fatalf("InstalledVariant() with invalid metadata = %q, want empty", got)
	}
}

func TestVerifyFilesPersistsInstalledVariant(t *testing.T) {
	dir := t.TempDir()
	dl := NewDownloader(dir, testLogger{})
	op := docker.NewOperation("prompt-guard", "install")

	for _, name := range []string{"model.onnx", "tokenizer.json"} {
		if err := osWriteFile(filepath.Join(dir, name), []byte("ok")); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	if err := dl.verifyFiles(op, Variants["86m"]); err != nil {
		t.Fatalf("verifyFiles: %v", err)
	}
	if got := dl.InstalledVariant(); got != "86m" {
		t.Fatalf("InstalledVariant() = %q, want 86m", got)
	}
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
