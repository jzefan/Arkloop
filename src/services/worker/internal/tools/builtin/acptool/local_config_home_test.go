package acptool

import (
	"path/filepath"
	"strings"
	"testing"

	"arkloop/services/worker/internal/acp"

	sharedtoolruntime "arkloop/services/shared/toolruntime"
	"github.com/google/uuid"
)

func TestMaybeInjectLocalOpenCodeConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", tmp)
	runID := uuid.New()
	env := map[string]string{}
	maybeInjectLocalOpenCodeConfigHome(acp.ResolvedProvider{ID: acp.DefaultProviderID, HostKind: acp.HostKindLocal}, nil, runID, env)
	got := env["XDG_CONFIG_HOME"]
	if got == "" {
		t.Fatal("expected XDG_CONFIG_HOME")
	}
	wantSuffix := filepath.Join("acp-runs", runID.String(), "config")
	if !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("path = %q, want suffix %q", got, wantSuffix)
	}
}

func TestMaybeInjectLocalOpenCodeConfigHomeSkipsSandbox(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", tmp)
	env := map[string]string{}
	maybeInjectLocalOpenCodeConfigHome(acp.ResolvedProvider{ID: acp.DefaultProviderID, HostKind: acp.HostKindSandbox}, nil, uuid.New(), env)
	if len(env) != 0 {
		t.Fatalf("unexpected env: %v", env)
	}
}

func TestMaybeInjectSkippedWhenConfigDeclaresXDGRaw(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", tmp)
	active := map[string]sharedtoolruntime.ProviderConfig{
		acp.ProviderGroupACP: {
			ConfigJSON: map[string]any{
				"env_overrides": map[string]any{"XDG_CONFIG_HOME": "/custom"},
			},
		},
	}
	env := map[string]string{}
	maybeInjectLocalOpenCodeConfigHome(acp.ResolvedProvider{ID: acp.DefaultProviderID, HostKind: acp.HostKindLocal}, active, uuid.New(), env)
	if len(env) != 0 {
		t.Fatalf("expected skip when env_overrides lists XDG_CONFIG_HOME, got %v", env)
	}
}
