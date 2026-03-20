package acptool

import (
	"os"
	"path/filepath"
	"strings"

	sharedtoolruntime "arkloop/services/shared/toolruntime"
	"arkloop/services/worker/internal/acp"
	"github.com/google/uuid"
)

func maybeInjectLocalOpenCodeConfigHome(
	provider acp.ResolvedProvider,
	active map[string]sharedtoolruntime.ProviderConfig,
	runID uuid.UUID,
	env map[string]string,
) {
	if provider.HostKind != acp.HostKindLocal || provider.ID != acp.DefaultProviderID {
		return
	}
	if _, exists := env["XDG_CONFIG_HOME"]; exists {
		return
	}
	if cfg, ok := active[acp.ProviderGroupACP]; ok && envOverrideHasKey(cfg.ConfigJSON, "XDG_CONFIG_HOME") {
		return
	}
	root, err := resolveACPUserDataRoot()
	if err != nil || strings.TrimSpace(root) == "" {
		return
	}
	dir := filepath.Join(root, "acp-runs", runID.String(), "config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	env["XDG_CONFIG_HOME"] = dir
}

func envOverrideHasKey(config map[string]any, key string) bool {
	if len(config) == 0 || strings.TrimSpace(key) == "" {
		return false
	}
	raw, ok := config["env_overrides"]
	if !ok || raw == nil {
		return false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, exists := m[key]
	return exists
}

func resolveACPUserDataRoot() (string, error) {
	if d := strings.TrimSpace(os.Getenv("ARKLOOP_DATA_DIR")); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".arkloop"), nil
}
