package app

import (
	"os"
	"testing"
)

func TestDefaultConfigSessionStateTTLDays(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SessionStateTTLDays != 7 {
		t.Fatalf("unexpected default ttl: %d", cfg.SessionStateTTLDays)
	}
}

func TestLoadConfigFromEnvSessionStateTTLDays(t *testing.T) {
	t.Setenv("ARKLOOP_SANDBOX_SESSION_STATE_TTL_DAYS", "0")
	t.Setenv("ARKLOOP_SANDBOX_ADDR", "127.0.0.1:8002")
	unsetSandboxConfigRegistryEnv(t)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.SessionStateTTLDays != 0 {
		t.Fatalf("unexpected ttl: %d", cfg.SessionStateTTLDays)
	}
}

func TestLoadConfigFromEnvSessionStateTTLDaysRejectNegative(t *testing.T) {
	t.Setenv("ARKLOOP_SANDBOX_SESSION_STATE_TTL_DAYS", "-1")
	t.Setenv("ARKLOOP_SANDBOX_ADDR", "127.0.0.1:8002")
	unsetSandboxConfigRegistryEnv(t)

	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected ttl validation error")
	}
}

func TestDefaultConfigAllowEgress(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.AllowEgress {
		t.Fatal("allow_egress should default to enabled")
	}
}

func TestLoadConfigFromEnvAllowEgress(t *testing.T) {
	t.Setenv("ARKLOOP_SANDBOX_ALLOW_EGRESS", "false")
	t.Setenv("ARKLOOP_SANDBOX_ADDR", "127.0.0.1:8002")
	unsetSandboxConfigRegistryEnv(t)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if cfg.AllowEgress {
		t.Fatal("expected allow_egress to be disabled")
	}
}

func TestLoadConfigFromEnvAllowEgressRejectInvalid(t *testing.T) {
	t.Setenv("ARKLOOP_SANDBOX_ALLOW_EGRESS", "maybe")
	t.Setenv("ARKLOOP_SANDBOX_ADDR", "127.0.0.1:8002")
	unsetSandboxConfigRegistryEnv(t)

	if _, err := LoadConfigFromEnv(); err == nil {
		t.Fatal("expected allow_egress validation error")
	}
}

func TestLoadConfigFromEnvFirecrackerDNS(t *testing.T) {
	t.Setenv("ARKLOOP_SANDBOX_ADDR", "127.0.0.1:8002")
	t.Setenv("ARKLOOP_SANDBOX_FIRECRACKER_DNS", "1.1.1.1, 8.8.8.8")
	unsetSandboxConfigRegistryEnv(t)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if len(cfg.FirecrackerDNS) != 2 {
		t.Fatalf("unexpected dns list: %#v", cfg.FirecrackerDNS)
	}
}

func unsetSandboxConfigRegistryEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ARKLOOP_DATABASE_URL"} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s failed: %v", key, err)
		}
	}
}
