package app

import "testing"

func TestDefaultConfigIncludesLocalCORSOrigins(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("unexpected default origins: %#v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "http://localhost:5173" || cfg.CORSAllowedOrigins[1] != "http://localhost:5174" {
		t.Fatalf("unexpected default origins: %#v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadConfigFromEnvParsesCORSAllowedOrigins(t *testing.T) {
	t.Setenv(corsAllowedOriginsEnv, "https://app.example.com, https://console.example.com")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("unexpected origins: %#v", cfg.CORSAllowedOrigins)
	}
	if cfg.CORSAllowedOrigins[0] != "https://app.example.com" || cfg.CORSAllowedOrigins[1] != "https://console.example.com" {
		t.Fatalf("unexpected origins: %#v", cfg.CORSAllowedOrigins)
	}
}
