package localproviders

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolverDetectsClaudeCodeAPIKeyFromGlobalConfig(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".claude.json"), map[string]any{
		"primaryApiKey": "sk-ant-local",
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeAPIKey || credential.APIKey != "sk-ant-local" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
}

func TestResolverDetectsCodexAPIKeyFromAuthJSON(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
		"OPENAI_API_KEY": "sk-local",
		"tokens":         nil,
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	credential, err := resolver.Resolve(context.Background(), CodexProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeAPIKey || credential.APIKey != "sk-local" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
}

func TestResolverCodexAPIKeyEnvPrecedesStoredAuth(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
		"OPENAI_API_KEY": "sk-file",
	})
	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{"CODEX_API_KEY": "sk-env", "OPENAI_API_KEY": "sk-ignored"},
	})
	credential, err := resolver.Resolve(context.Background(), CodexProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeAPIKey || credential.APIKey != "sk-env" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
}

func TestResolverCodexIgnoresOpenAIEnvForAuthPrecedence(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
		"OPENAI_API_KEY": "sk-file",
	})

	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{"OPENAI_API_KEY": "sk-env"},
	})
	credential, err := resolver.Resolve(context.Background(), CodexProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeAPIKey || credential.APIKey != "sk-file" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
}

func TestResolverClaudeExternalAPIKeyPrecedesOAuthStore(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".claude", ".credentials.json"), map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "oauth-access",
			"refreshToken": "oauth-refresh",
			"expiresAt":    time.Now().Add(time.Hour).UnixMilli(),
		},
	})

	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{"ANTHROPIC_API_KEY": "sk-env"},
	})
	credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeAPIKey || credential.APIKey != "sk-env" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
}

func TestResolverDetectsClaudeCodeSettingsEnv(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"env": map[string]any{
			"ANTHROPIC_AUTH_TOKEN":           "oauth-local",
			"ANTHROPIC_BASE_URL":             "https://gateway.local",
			"ANTHROPIC_MODEL":                "accounts/fireworks/models/deepseek-v4-pro",
			"ANTHROPIC_REASONING_MODEL":      "accounts/fireworks/models/deepseek-v4-pro",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "accounts/fireworks/models/deepseek-v4-pro",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "accounts/fireworks/models/deepseek-v4-pro",
			"ANTHROPIC_DEFAULT_OPUS_MODEL":   "accounts/fireworks/models/deepseek-v4-pro",
		},
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeOAuth || credential.AccessToken != "oauth-local" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
	if credential.BaseURL != "https://gateway.local" {
		t.Fatalf("expected base url from Claude Code settings, got %q", credential.BaseURL)
	}

	statuses := resolver.ProviderStatuses(context.Background())
	if len(statuses) != 1 || statuses[0].ID != ClaudeCodeProviderID {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	models := statuses[0].Models
	if len(models) != 1 || models[0].ID != "accounts/fireworks/models/deepseek-v4-pro" || !models[0].Default {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestResolverClaudeProcessEnvPrecedesSettingsEnv(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"env": map[string]any{
			"ANTHROPIC_AUTH_TOKEN": "settings-token",
			"ANTHROPIC_BASE_URL":   "https://settings.local",
		},
	})

	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env: map[string]string{
			"ANTHROPIC_AUTH_TOKEN": "env-token",
			"ANTHROPIC_BASE_URL":   "https://env.local",
		},
	})
	credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeOAuth || credential.AccessToken != "env-token" {
		t.Fatalf("unexpected credential: %#v", credential)
	}
	if credential.BaseURL != "https://env.local" {
		t.Fatalf("expected base url from process env, got %q", credential.BaseURL)
	}
}

func TestResolverClaudeAPIKeyHelperBlocksManagedStores(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".claude", "settings.json"), map[string]any{
		"apiKeyHelper": "helper-command",
	})
	writeTestJSON(t, filepath.Join(home, ".claude", ".credentials.json"), map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "oauth-access",
			"refreshToken": "oauth-refresh",
			"expiresAt":    time.Now().Add(time.Hour).UnixMilli(),
		},
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	_, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if !errors.Is(err, ErrCredentialUnavailable) {
		t.Fatalf("expected unavailable from apiKeyHelper precedence, got %v", err)
	}
}

func TestResolverHonorsCodexExplicitChatGPTMode(t *testing.T) {
	home := t.TempDir()
	accessToken := testJWT(t, time.Now().Add(time.Hour), map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc_123"},
	})
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": "sk-ignored",
		"tokens": map[string]any{
			"access_token":  accessToken,
			"refresh_token": "refresh-local",
		},
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	credential, err := resolver.Resolve(context.Background(), CodexProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeOAuth || credential.APIKey != "" || credential.AccessToken != accessToken {
		t.Fatalf("unexpected credential: %#v", credential)
	}
	if credential.AccountID != "acc_123" {
		t.Fatalf("expected account id from token, got %q", credential.AccountID)
	}
}

func TestResolverRefreshesClaudeOAuthAndWritesBack(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(2_000_000_000, 0)
	credentialsPath := filepath.Join(home, ".claude", ".credentials.json")
	writeTestJSON(t, credentialsPath, map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "old-access",
			"refreshToken": "refresh-old",
			"expiresAt":    now.Add(-time.Hour).UnixMilli(),
			"scopes":       []any{"user:inference"},
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if payload["refresh_token"] != "refresh-old" || payload["scope"] != claudeOAuthScopes {
			t.Fatalf("unexpected refresh payload: %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "refresh-new",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{},
		HTTPClient:      server.Client(),
		Now:             func() time.Time { return now },
	})
	withClaudeRefreshURLForTest(t, server.URL, func() {
		credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{Refresh: true})
		if err != nil {
			t.Fatalf("Resolve refresh: %v", err)
		}
		if credential.AccessToken != "new-access" || credential.RefreshToken != "refresh-new" {
			t.Fatalf("unexpected refreshed credential: %#v", credential)
		}
	})

	root, ok := readJSONFile(credentialsPath)
	if !ok {
		t.Fatalf("credentials json missing")
	}
	oauth, _ := root["claudeAiOauth"].(map[string]any)
	if oauth["accessToken"] != "new-access" || oauth["refreshToken"] != "refresh-new" {
		t.Fatalf("refresh was not written back: %#v", oauth)
	}
}

func TestResolverRefreshesCodexOAuthAndWritesBack(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(2_000_000_000, 0)
	oldAccess := testJWT(t, now.Add(-time.Hour), nil)
	newAccess := testJWT(t, now.Add(time.Hour), map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acc_new"},
	})
	authPath := filepath.Join(home, ".codex", "auth.json")
	writeTestJSON(t, authPath, map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token":  oldAccess,
			"refresh_token": "refresh-old",
		},
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.Form.Get("refresh_token") != "refresh-old" {
			t.Fatalf("unexpected refresh token")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  newAccess,
			"refresh_token": "refresh-new",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{},
		HTTPClient:      server.Client(),
		Now:             func() time.Time { return now },
	})
	withCodexRefreshURLForTest(t, server.URL, func() {
		credential, err := resolver.Resolve(context.Background(), CodexProviderID, ResolveOptions{Refresh: true})
		if err != nil {
			t.Fatalf("Resolve refresh: %v", err)
		}
		if credential.AccessToken != newAccess || credential.RefreshToken != "refresh-new" || credential.AccountID != "acc_new" {
			t.Fatalf("unexpected refreshed credential: %#v", credential)
		}
	})

	root, ok := readJSONFile(authPath)
	if !ok {
		t.Fatalf("auth json missing")
	}
	tokens, _ := root["tokens"].(map[string]any)
	if tokens["access_token"] != newAccess || tokens["refresh_token"] != "refresh-new" {
		t.Fatalf("refresh was not written back: %#v", tokens)
	}
}

func TestResolverUsesClaudeConfigDirKeychainServiceHash(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, "custom-claude")
	var seenService string
	resolver := NewResolver(Options{
		HomeDir:  home,
		Platform: "darwin",
		Env:      map[string]string{"CLAUDE_CONFIG_DIR": configDir},
		CommandRunner: func(ctx context.Context, name string, args ...string) (string, error) {
			if name != "security" {
				t.Fatalf("unexpected command: %s", name)
			}
			for i, arg := range args {
				if arg == "-s" && i+1 < len(args) {
					seenService = args[i+1]
				}
			}
			return `{"claudeAiOauth":{"accessToken":"access","refreshToken":"refresh","expiresAt":4102444800000}}`, nil
		},
	})
	credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.AuthMode != AuthModeOAuth {
		t.Fatalf("unexpected credential: %#v", credential)
	}
	if !strings.HasPrefix(seenService, "Claude Code-credentials-") || len(seenService) != len("Claude Code-credentials-")+8 {
		t.Fatalf("unexpected service name: %q", seenService)
	}
}

func TestResolverCacheAvoidsRepeatedKeychainReads(t *testing.T) {
	home := t.TempDir()
	var reads atomic.Int32
	resolver := NewResolver(Options{
		HomeDir:  home,
		Platform: "darwin",
		Env:      map[string]string{},
		CommandRunner: func(ctx context.Context, name string, args ...string) (string, error) {
			if name != "security" {
				t.Fatalf("unexpected command: %s", name)
			}
			reads.Add(1)
			return `{"claudeAiOauth":{"accessToken":"access","refreshToken":"refresh","expiresAt":4102444800000}}`, nil
		},
	})
	for i := 0; i < 2; i++ {
		credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
		if err != nil {
			t.Fatalf("Resolve %d: %v", i, err)
		}
		if credential.AuthMode != AuthModeOAuth || credential.AccessToken != "access" {
			t.Fatalf("unexpected credential: %#v", credential)
		}
	}
	if reads.Load() != 1 {
		t.Fatalf("expected one keychain read, got %d", reads.Load())
	}
}

func TestResolverFileFingerprintInvalidatesCachedCredential(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	writeTestJSON(t, path, map[string]any{"primaryApiKey": "sk-one"})

	now := time.Unix(2_000_000_000, 0)
	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{},
		Now:             func() time.Time { return now },
	})
	credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve initial: %v", err)
	}
	if credential.APIKey != "sk-one" {
		t.Fatalf("unexpected initial credential: %#v", credential)
	}

	writeTestJSON(t, path, map[string]any{"primaryApiKey": "sk-two-longer"})
	nextMtime := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, nextMtime, nextMtime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	credential, err = resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve updated: %v", err)
	}
	if credential.APIKey != "sk-two-longer" {
		t.Fatalf("expected fingerprint invalidation, got %#v", credential)
	}
}

func TestResolverRefreshesClaudeOAuthOnceInFlightAndWritesBack(t *testing.T) {
	home := t.TempDir()
	now := time.Unix(2_000_000_000, 0)
	credentialsPath := filepath.Join(home, ".claude", ".credentials.json")
	writeTestJSON(t, credentialsPath, map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  "old-access",
			"refreshToken": "refresh-old",
			"expiresAt":    now.Add(-time.Hour).UnixMilli(),
		},
	})

	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "refresh-new",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	resolver := NewResolver(Options{
		HomeDir:         home,
		DisableKeychain: true,
		Env:             map[string]string{},
		HTTPClient:      server.Client(),
		Now:             func() time.Time { return now },
	})
	withClaudeRefreshURLForTest(t, server.URL, func() {
		var wg sync.WaitGroup
		results := make(chan Credential, 2)
		errs := make(chan error, 2)
		resolve := func() {
			defer wg.Done()
			credential, err := resolver.Resolve(context.Background(), ClaudeCodeProviderID, ResolveOptions{Refresh: true})
			if err != nil {
				errs <- err
				return
			}
			results <- credential
		}
		wg.Add(1)
		go resolve()
		<-started
		wg.Add(1)
		go resolve()
		close(release)
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			t.Fatalf("Resolve refresh: %v", err)
		}
		for credential := range results {
			if credential.AccessToken != "new-access" || credential.RefreshToken != "refresh-new" {
				t.Fatalf("unexpected refreshed credential: %#v", credential)
			}
		}
	})

	if calls.Load() != 1 {
		t.Fatalf("expected one refresh request, got %d", calls.Load())
	}
	root, ok := readJSONFile(credentialsPath)
	if !ok {
		t.Fatalf("credentials json missing")
	}
	oauth, _ := root["claudeAiOauth"].(map[string]any)
	if oauth["accessToken"] != "new-access" || oauth["refreshToken"] != "refresh-new" {
		t.Fatalf("refresh was not written back: %#v", oauth)
	}
}

func TestResolverKeychainWriteUsesCallerContext(t *testing.T) {
	type contextKey string
	const marker contextKey = "marker"

	home := t.TempDir()
	now := time.Unix(2_000_000_000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "refresh-new",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(server.Close)

	resolver := NewResolver(Options{
		HomeDir:    home,
		Platform:   "darwin",
		Env:        map[string]string{},
		HTTPClient: server.Client(),
		Now:        func() time.Time { return now },
		CommandRunner: func(ctx context.Context, name string, args ...string) (string, error) {
			if name != "security" {
				t.Fatalf("unexpected command: %s", name)
			}
			if len(args) > 0 && args[0] == "add-generic-password" {
				if ctx.Value(marker) != "ok" {
					return "", errors.New("missing caller context")
				}
				return "", nil
			}
			return `{"claudeAiOauth":{"accessToken":"old-access","refreshToken":"refresh-old","expiresAt":1}}`, nil
		},
	})

	ctx := context.WithValue(context.Background(), marker, "ok")
	withClaudeRefreshURLForTest(t, server.URL, func() {
		if _, err := resolver.Resolve(ctx, ClaudeCodeProviderID, ResolveOptions{Refresh: true}); err != nil {
			t.Fatalf("Resolve refresh: %v", err)
		}
	})
}

func TestProviderStatusesReturnNoSecrets(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".claude.json"), map[string]any{"primaryApiKey": "sk-ant-local"})
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{"OPENAI_API_KEY": "sk-local"})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	statuses := resolver.ProviderStatuses(context.Background())
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	raw, _ := json.Marshal(statuses)
	if strings.Contains(string(raw), "sk-") {
		t.Fatalf("provider status leaked credential: %s", raw)
	}
}

func TestProviderStatusesUseCodexModelCatalog(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{"OPENAI_API_KEY": "sk-local"})
	writeTestJSON(t, filepath.Join(home, ".codex", "model-catalog.test.json"), map[string]any{
		"models": []any{
			map[string]any{"slug": "codex-auto-review", "visibility": "hide", "priority": 1},
			map[string]any{"slug": "gpt-5.5", "visibility": "list", "priority": 0, "context_window": 272000},
			map[string]any{"slug": "gpt-5.3-codex-spark", "visibility": "list", "priority": 2, "context_window": 128000},
		},
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	statuses := resolver.ProviderStatuses(context.Background())
	if len(statuses) != 1 || statuses[0].ID != CodexProviderID {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	models := statuses[0].Models
	if len(models) != 2 {
		t.Fatalf("expected hidden model to be skipped, got %#v", models)
	}
	if models[0].ID != "gpt-5.5" || !models[0].Default || models[0].ContextLength != 272000 {
		t.Fatalf("unexpected first model: %#v", models[0])
	}
	if models[1].ID != "gpt-5.3-codex-spark" || models[1].ContextLength != 128000 {
		t.Fatalf("unexpected second model: %#v", models[1])
	}
}

func TestProviderStatusesApplyLocalModelVisibility(t *testing.T) {
	home := t.TempDir()
	writeTestJSON(t, filepath.Join(home, ".codex", "auth.json"), map[string]any{"OPENAI_API_KEY": "sk-local"})
	writeTestJSON(t, filepath.Join(home, ".codex", "model-catalog.test.json"), map[string]any{
		"models": []any{
			map[string]any{"slug": "gpt-5.5", "visibility": "list", "priority": 0, "context_window": 272000},
			map[string]any{"slug": "gpt-5.4", "visibility": "list", "priority": 1, "context_window": 272000},
		},
	})

	resolver := NewResolver(Options{HomeDir: home, DisableKeychain: true, Env: map[string]string{}})
	if err := resolver.SetModelVisible(CodexProviderID, "gpt-5.5", false); err != nil {
		t.Fatalf("SetModelVisible: %v", err)
	}
	statuses := resolver.ProviderStatuses(context.Background())
	if len(statuses) != 1 || len(statuses[0].Models) != 2 {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	if !statuses[0].Models[0].Hidden || statuses[0].Models[0].Default {
		t.Fatalf("expected first model hidden and non-default: %#v", statuses[0].Models[0])
	}
	if statuses[0].Models[1].Hidden || !statuses[0].Models[1].Default {
		t.Fatalf("expected second model promoted to default: %#v", statuses[0].Models[1])
	}

	if err := resolver.SetModelVisible(CodexProviderID, "gpt-5.5", true); err != nil {
		t.Fatalf("SetModelVisible true: %v", err)
	}
	statuses = resolver.ProviderStatuses(context.Background())
	if statuses[0].Models[0].Hidden {
		t.Fatalf("expected first model visible again: %#v", statuses[0].Models[0])
	}
}

func writeTestJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func testJWT(t *testing.T, exp time.Time, extra map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "none"}
	payload := map[string]any{"exp": exp.Unix()}
	for key, value := range extra {
		payload[key] = value
	}
	headerRaw, _ := json.Marshal(header)
	payloadRaw, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(headerRaw) + "." + base64.RawURLEncoding.EncodeToString(payloadRaw) + "."
}

func withCodexRefreshURLForTest(t *testing.T, url string, run func()) {
	t.Helper()
	previous := codexOAuthTokenEndpoint
	codexOAuthTokenEndpoint = url
	t.Cleanup(func() {
		codexOAuthTokenEndpoint = previous
	})
	run()
}

func withClaudeRefreshURLForTest(t *testing.T, url string, run func()) {
	t.Helper()
	previous := claudeOAuthTokenEndpoint
	claudeOAuthTokenEndpoint = url
	t.Cleanup(func() {
		claudeOAuthTokenEndpoint = previous
	})
	run()
}
