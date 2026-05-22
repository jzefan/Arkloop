//go:build !desktop

package app

import "testing"

func TestLoadConfigFromEnvLoadsKBDebugEmbeddingSettings(t *testing.T) {
	t.Setenv(kbDebugTokenEnv, "debug-token")
	t.Setenv(arkAPIKeyFallbackEnv, "doubao-key")
	t.Setenv(arkBaseURLFallbackEnv, "https://example.test/api/v3")
	t.Setenv(arkEmbedModelEnv, "endpoint-id")
	t.Setenv(arkEmbedBatchEnv, "7")
	t.Setenv(arkEmbedDimEnv, "1024")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}

	if cfg.KBDebugToken != "debug-token" {
		t.Fatalf("KBDebugToken=%q", cfg.KBDebugToken)
	}
	if cfg.DoubaoEmbedAPIKey != "doubao-key" {
		t.Fatalf("DoubaoEmbedAPIKey not loaded from fallback env")
	}
	if cfg.DoubaoEmbedBaseURL != "https://example.test/api/v3" {
		t.Fatalf("DoubaoEmbedBaseURL=%q", cfg.DoubaoEmbedBaseURL)
	}
	if cfg.DoubaoEmbedModel != "endpoint-id" {
		t.Fatalf("DoubaoEmbedModel=%q", cfg.DoubaoEmbedModel)
	}
	if cfg.DoubaoEmbedBatchSize != 7 {
		t.Fatalf("DoubaoEmbedBatchSize=%d", cfg.DoubaoEmbedBatchSize)
	}
	if cfg.DoubaoEmbedDim != 1024 {
		t.Fatalf("DoubaoEmbedDim=%d", cfg.DoubaoEmbedDim)
	}
}

func TestLoadConfigFromEnvRejectsInvalidEmbedDim(t *testing.T) {
	t.Setenv(arkEmbedDimEnv, "0")

	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected invalid embed dim error")
	}
}
