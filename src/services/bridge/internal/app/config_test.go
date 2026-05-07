package app

import (
	"os"
	"testing"
)

func TestLoadConfigFromEnv_AppendsCustomCORSOrigins(t *testing.T) {
	t.Setenv(bridgeCORSOriginsEnv, "http://localhost:5174")
	t.Setenv(bridgeProjectDirEnv, t.TempDir())
	t.Setenv(bridgeModulesFileEnv, t.TempDir())
	t.Setenv(bridgeAuthTokenEnv, "test-token")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}

	foundDefault := false
	foundCustom := false
	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "http://localhost:19080" {
			foundDefault = true
		}
		if origin == "http://localhost:5174" {
			foundCustom = true
		}
	}

	if !foundDefault {
		t.Fatalf("expected default bridge CORS origin to be preserved, got %#v", cfg.CORSAllowedOrigins)
	}
	if !foundCustom {
		t.Fatalf("expected custom bridge CORS origin to be appended, got %#v", cfg.CORSAllowedOrigins)
	}
}

func TestDefaultConfigIncludesViteDevCORSOrigins(t *testing.T) {
	cfg := DefaultConfig()

	foundLocalhost := false
	foundLoopback := false
	for _, origin := range cfg.CORSAllowedOrigins {
		if origin == "http://localhost:5173" {
			foundLocalhost = true
		}
		if origin == "http://127.0.0.1:5173" {
			foundLoopback = true
		}
	}

	if !foundLocalhost || !foundLoopback {
		t.Fatalf("expected Vite dev origins in defaults, got %#v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadConfigFromEnv_RequiresAuthToken(t *testing.T) {
	t.Setenv(bridgeAuthTokenEnv, "")
	t.Setenv(bridgeProjectDirEnv, t.TempDir())
	t.Setenv(bridgeModulesFileEnv, t.TempDir())

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected error when auth token is empty")
	}
}

func TestLoadConfigFromEnv_AllowsLoopbackAddrWithoutRepoDetection(t *testing.T) {
	t.Setenv(bridgeAddrEnv, "127.0.0.1:19007")
	t.Setenv(bridgeProjectDirEnv, "")
	t.Setenv(bridgeModulesFileEnv, "")
	t.Setenv(bridgeAuthTokenEnv, "test-token")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv() error = %v", err)
	}
	if cfg.Addr != "127.0.0.1:19007" {
		t.Fatalf("expected loopback addr to be preserved, got %q", cfg.Addr)
	}
}
