package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Per-user MCP auth.
//
// Some upstream MCP servers (notably the exam adapter) must run "as the current
// teacher" so the upstream can enforce per-user permission / org isolation /
// audit. The shared, pooled MCP client carries only static install-level auth,
// so per-user identity is injected per CallTool via the request context: the
// executor mints a short-lived user token (reusing ArkLoop's /internal/oauth/issue
// OIDC bridge) and stamps it into ctx; the HTTP transport applies it as the
// Authorization header for that single request.
//
// This only works over HTTP transports (per-call header). stdio fixes its auth
// at process spawn and cannot do per-call identity.

// UserTokenMinter mints a short-lived bearer token authenticating an upstream
// call as a specific ArkLoop user.
type UserTokenMinter interface {
	Mint(ctx context.Context, userID uuid.UUID) (string, error)
}

var (
	perUserMu      sync.RWMutex
	perUserOnce    sync.Once
	perUserMinter  UserTokenMinter
	perUserServers map[string]bool
)

// ConfigurePerUserAuth explicitly sets the minter and the set of server IDs that
// require per-user OIDC injection. Mainly for wiring/tests; when never called,
// configuration is loaded lazily from the environment on first use.
func ConfigurePerUserAuth(minter UserTokenMinter, serverIDs []string) {
	perUserMu.Lock()
	defer perUserMu.Unlock()
	perUserMinter = minter
	perUserServers = map[string]bool{}
	for _, id := range serverIDs {
		if id = strings.TrimSpace(id); id != "" {
			perUserServers[id] = true
		}
	}
}

func loadPerUserAuthFromEnv() {
	minter := newOIDCMinterFromEnv()
	servers := map[string]bool{}
	for _, id := range splitList(os.Getenv("ARKLOOP_MCP_OIDC_SERVERS")) {
		servers[id] = true
	}
	perUserMu.Lock()
	defer perUserMu.Unlock()
	if perUserMinter == nil {
		perUserMinter = minter
	}
	if perUserServers == nil {
		perUserServers = servers
	}
}

// perUserAuthFor returns a minter when serverID requires per-user auth and a
// minter is configured; otherwise nil (caller keeps static install auth).
func perUserAuthFor(serverID string) UserTokenMinter {
	perUserOnce.Do(loadPerUserAuthFromEnv)
	perUserMu.RLock()
	defer perUserMu.RUnlock()
	if perUserMinter == nil || len(perUserServers) == 0 {
		return nil
	}
	if perUserServers[strings.TrimSpace(serverID)] {
		return perUserMinter
	}
	return nil
}

// --- context-carried per-call auth override ---

type authOverrideKey struct{}

func withAuthOverride(ctx context.Context, bearer string) context.Context {
	return context.WithValue(ctx, authOverrideKey{}, bearer)
}

func authOverrideFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(authOverrideKey{}).(string)
	return v, ok && strings.TrimSpace(v) != ""
}

// --- OIDC minter (POST /internal/oauth/issue) ---

type oidcMinter struct {
	apiBaseURL   string
	serviceToken string
	clientID     string
	scopes       []string
	httpClient   *http.Client
}

// newOIDCMinterFromEnv builds a minter from env, or returns nil when the
// internal service token is absent (per-user auth then stays disabled).
func newOIDCMinterFromEnv() UserTokenMinter {
	service := strings.TrimSpace(os.Getenv("ARKLOOP_INTERNAL_SERVICE_TOKEN"))
	if service == "" {
		return nil
	}
	base := strings.TrimSpace(os.Getenv("ARKLOOP_API_INTERNAL_URL"))
	if base == "" {
		base = "http://api:19001"
	}
	clientID := strings.TrimSpace(os.Getenv("ARKLOOP_MCP_OIDC_CLIENT_ID"))
	if clientID == "" {
		clientID = "exam-web"
	}
	scopes := splitList(os.Getenv("ARKLOOP_MCP_OIDC_SCOPES"))
	if len(scopes) == 0 {
		scopes = []string{"openid", "exam:read", "exam:write"}
	}
	return &oidcMinter{
		apiBaseURL:   strings.TrimRight(base, "/"),
		serviceToken: service,
		clientID:     clientID,
		scopes:       scopes,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (m *oidcMinter) Mint(ctx context.Context, userID uuid.UUID) (string, error) {
	if userID == uuid.Nil {
		return "", fmt.Errorf("mcp: per-user auth requires a user context")
	}
	body, _ := json.Marshal(map[string]any{
		"user_id":   userID.String(),
		"client_id": m.clientID,
		"scopes":    m.scopes,
	})
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, m.apiBaseURL+"/internal/oauth/issue", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.serviceToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("mcp: mint user token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mcp: mint user token status=%d: %s", resp.StatusCode, truncateStr(string(raw), 200))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("mcp: decode mint response: %w", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", fmt.Errorf("mcp: mint returned empty access_token")
	}
	return out.AccessToken, nil
}

func splitList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
