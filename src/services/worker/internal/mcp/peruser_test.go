package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

// --- minter ---

func TestOIDCMinter_Mint(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "minted-jwt", "token_type": "Bearer", "expires_in": 60,
		})
	}))
	defer srv.Close()

	m := &oidcMinter{
		apiBaseURL: srv.URL, serviceToken: "svc-tok",
		clientID: "exam-web", scopes: []string{"openid", "exam:read"},
		httpClient: srv.Client(),
	}
	uid := uuid.New()
	tok, err := m.Mint(context.Background(), uid)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if tok != "minted-jwt" {
		t.Fatalf("token = %q", tok)
	}
	if gotAuth != "Bearer svc-tok" {
		t.Errorf("service auth = %q, want Bearer svc-tok", gotAuth)
	}
	if gotBody["user_id"] != uid.String() {
		t.Errorf("user_id = %v, want %s", gotBody["user_id"], uid)
	}
	if gotBody["client_id"] != "exam-web" {
		t.Errorf("client_id = %v", gotBody["client_id"])
	}
}

func TestOIDCMinter_RejectsNilUser(t *testing.T) {
	m := &oidcMinter{apiBaseURL: "http://unused", serviceToken: "s", httpClient: http.DefaultClient}
	if _, err := m.Mint(context.Background(), uuid.Nil); err == nil {
		t.Fatal("expected error for nil user")
	}
}

// --- sendHTTP applies the per-call auth override ---

func TestSendHTTPAppliesAuthOverride(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["method"] == "tools/call" {
			gotAuth = r.Header.Get("Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": payload["id"],
			"result": map[string]any{"content": []any{}, "isError": false},
		})
	}))
	defer srv.Close()

	client := &HTTPClient{
		server: ServerConfig{
			Transport: "streamable_http", URL: srv.URL,
			Headers: map[string]string{}, CallTimeoutMs: 1000,
		},
		httpClient: srv.Client(),
	}
	client.nextID.Store(1)
	client.initialized = true // skip handshake; exercise tools/call only

	ctx := withAuthOverride(context.Background(), "user-token-xyz")
	if _, err := client.CallTool(ctx, "remote_tool", map[string]any{}, 1000); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if gotAuth != "Bearer user-token-xyz" {
		t.Fatalf("Authorization = %q, want Bearer user-token-xyz", gotAuth)
	}

	// Without an override, no Authorization is sent.
	gotAuth = ""
	if _, err := client.CallTool(context.Background(), "remote_tool", map[string]any{}, 1000); err != nil {
		t.Fatalf("CallTool(plain): %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization without override, got %q", gotAuth)
	}
}

// --- executor injects per-user auth for allowlisted servers ---

type fakeMinter struct {
	token   string
	gotUser uuid.UUID
}

func (m *fakeMinter) Mint(_ context.Context, userID uuid.UUID) (string, error) {
	m.gotUser = userID
	return m.token, nil
}

type recordingClient struct {
	gotCtx  context.Context
	gotName string
}

func (c *recordingClient) ListTools(context.Context, int) ([]Tool, error) { return nil, nil }
func (c *recordingClient) CallTool(ctx context.Context, name string, _ map[string]any, _ int) (ToolCallResult, error) {
	c.gotCtx = ctx
	c.gotName = name
	return ToolCallResult{Content: []map[string]any{{"type": "text", "text": "ok"}}}, nil
}
func (c *recordingClient) IsHealthy(context.Context) bool { return true }
func (c *recordingClient) Close() error                   { return nil }

func newExecWithFakeClient(t *testing.T, serverID string, rec *recordingClient) *ToolExecutor {
	t.Helper()
	pool := NewPool()
	server := ServerConfig{ServerID: serverID, AccountID: "acct-1", Transport: "streamable_http", CallTimeoutMs: 1000}
	pool.clients[poolKey(server.AccountID, server.ServerID)] = rec
	return NewToolExecutor(server, map[string]string{"tool1": "remote_tool"}, pool)
}

func TestExecuteInjectsPerUserAuth(t *testing.T) {
	fm := &fakeMinter{token: "minted-tok"}
	ConfigurePerUserAuth(fm, []string{"exam-agent"})
	defer ConfigurePerUserAuth(nil, nil)

	rec := &recordingClient{}
	exec := newExecWithFakeClient(t, "exam-agent", rec)
	uid := uuid.New()

	res := exec.Execute(context.Background(), "tool1", map[string]any{}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	if rec.gotName != "remote_tool" {
		t.Fatalf("remote tool = %q", rec.gotName)
	}
	tok, ok := authOverrideFromContext(rec.gotCtx)
	if !ok || tok != "minted-tok" {
		t.Fatalf("injected token = %q (ok=%v), want minted-tok", tok, ok)
	}
	if fm.gotUser != uid {
		t.Fatalf("minter saw user %v, want %v", fm.gotUser, uid)
	}
}

func TestExecuteNoPerUserWhenNotAllowlisted(t *testing.T) {
	ConfigurePerUserAuth(&fakeMinter{token: "x"}, []string{"some-other-server"})
	defer ConfigurePerUserAuth(nil, nil)

	rec := &recordingClient{}
	exec := newExecWithFakeClient(t, "exam-agent", rec)
	uid := uuid.New()

	res := exec.Execute(context.Background(), "tool1", map[string]any{}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	if _, ok := authOverrideFromContext(rec.gotCtx); ok {
		t.Fatalf("did not expect auth override for non-allowlisted server")
	}
}
