package http

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"testing"
	"time"

	"arkloop/services/api/internal/auth"
	apiCrypto "arkloop/services/api/internal/crypto"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"
)

type toolProvidersListResponse struct {
	Groups []toolProvidersGroup `json:"groups"`
}

type toolProvidersGroup struct {
	GroupName string                 `json:"group_name"`
	Providers []toolProviderListItem `json:"providers"`
}

type toolProviderListItem struct {
	GroupName       string  `json:"group_name"`
	ProviderName    string  `json:"provider_name"`
	IsActive        bool    `json:"is_active"`
	KeyPrefix       *string `json:"key_prefix"`
	BaseURL         *string `json:"base_url"`
	RequiresAPIKey  bool    `json:"requires_api_key"`
	RequiresBaseURL bool    `json:"requires_base_url"`
	Configured      bool    `json:"configured"`
}

func TestToolProvidersListActivateCredentialAndClear(t *testing.T) {
	db := setupTestDatabase(t, "api_go_tool_providers")

	ctx := context.Background()
	pool, err := data.NewPool(ctx, db.DSN)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := observability.NewJSONLogger("test", io.Discard)

	userRepo, err := data.NewUserRepository(pool)
	if err != nil {
		t.Fatalf("user repo: %v", err)
	}
	credRepo, err := data.NewUserCredentialRepository(pool)
	if err != nil {
		t.Fatalf("cred repo: %v", err)
	}
	membershipRepo, err := data.NewOrgMembershipRepository(pool)
	if err != nil {
		t.Fatalf("membership repo: %v", err)
	}
	refreshTokenRepo, err := data.NewRefreshTokenRepository(pool)
	if err != nil {
		t.Fatalf("refresh repo: %v", err)
	}
	orgRepo, err := data.NewOrgRepository(pool)
	if err != nil {
		t.Fatalf("org repo: %v", err)
	}
	toolProvidersRepo, err := data.NewToolProviderConfigsRepository(pool)
	if err != nil {
		t.Fatalf("tool providers repo: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 3)
	}
	ring, err := apiCrypto.NewKeyRing(map[int][]byte{1: key})
	if err != nil {
		t.Fatalf("new key ring: %v", err)
	}
	secretsRepo, err := data.NewSecretsRepository(pool, ring)
	if err != nil {
		t.Fatalf("secrets repo: %v", err)
	}

	passwordHasher, err := auth.NewBcryptPasswordHasher(0)
	if err != nil {
		t.Fatalf("new password hasher: %v", err)
	}
	tokenService, err := auth.NewJwtAccessTokenService("test-secret-should-be-long-enough-32chars", 3600, 7776000)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	authService, err := auth.NewService(userRepo, credRepo, membershipRepo, passwordHasher, tokenService, refreshTokenRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	org, err := orgRepo.Create(ctx, "tool-providers-org", "Tool Providers Org", "personal")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	user, err := userRepo.Create(ctx, "admin", "admin@test.com", "en")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := membershipRepo.Create(ctx, org.ID, user.ID, auth.RolePlatformAdmin); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	token, err := tokenService.Issue(user.ID, org.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	handler := NewHandler(HandlerConfig{
		Pool:                    pool,
		DirectPool:              pool,
		Logger:                  logger,
		AuthService:             authService,
		OrgMembershipRepo:       membershipRepo,
		ToolProviderConfigsRepo: toolProvidersRepo,
		SecretsRepo:             secretsRepo,
	})

	// 初始列表
	listResp := doJSON(handler, nethttp.MethodGet, "/v1/tool-providers", nil, authHeader(token))
	if listResp.Code != nethttp.StatusOK {
		t.Fatalf("list: %d %s", listResp.Code, listResp.Body.String())
	}
	initial := decodeJSONBody[toolProvidersListResponse](t, listResp.Body.Bytes())
	if len(initial.Groups) == 0 {
		t.Fatal("expected groups, got 0")
	}

	// 激活 tavily
	actTavily := doJSON(handler, nethttp.MethodPut, "/v1/tool-providers/web_search/web_search.tavily/activate", nil, authHeader(token))
	if actTavily.Code != nethttp.StatusNoContent {
		t.Fatalf("activate tavily: %d %s", actTavily.Code, actTavily.Body.String())
	}

	// 同组切换到 searxng
	actSearx := doJSON(handler, nethttp.MethodPut, "/v1/tool-providers/web_search/web_search.searxng/activate", nil, authHeader(token))
	if actSearx.Code != nethttp.StatusNoContent {
		t.Fatalf("activate searxng: %d %s", actSearx.Code, actSearx.Body.String())
	}

	listAfterActivate := doJSON(handler, nethttp.MethodGet, "/v1/tool-providers", nil, authHeader(token))
	if listAfterActivate.Code != nethttp.StatusOK {
		t.Fatalf("list after activate: %d %s", listAfterActivate.Code, listAfterActivate.Body.String())
	}
	afterActivate := decodeJSONBody[toolProvidersListResponse](t, listAfterActivate.Body.Bytes())

	var tavilyActive, searxActive bool
	for _, g := range afterActivate.Groups {
		if g.GroupName != "web_search" {
			continue
		}
		for _, p := range g.Providers {
			if p.ProviderName == "web_search.tavily" {
				tavilyActive = p.IsActive
			}
			if p.ProviderName == "web_search.searxng" {
				searxActive = p.IsActive
			}
		}
	}
	if tavilyActive {
		t.Fatal("expected tavily inactive after switching")
	}
	if !searxActive {
		t.Fatal("expected searxng active after switching")
	}

	// 预置 config_json，确保后续仅更新凭证时不会被覆盖成 {}
	if _, err := pool.Exec(ctx, `
		UPDATE tool_provider_configs
		SET config_json = '{"keep": true}'::jsonb
		WHERE org_id = $1 AND provider_name = 'web_search.tavily'
	`, org.ID); err != nil {
		t.Fatalf("seed config_json: %v", err)
	}

	// 配置 tavily key（不激活也应允许配置）
	keyPayload := map[string]any{"api_key": "tvly-1234567890abcdef"}
	upsert := doJSON(handler, nethttp.MethodPut, "/v1/tool-providers/web_search/web_search.tavily/credential", keyPayload, authHeader(token))
	if upsert.Code != nethttp.StatusNoContent {
		t.Fatalf("upsert credential: %d %s", upsert.Code, upsert.Body.String())
	}

	var rawCfg []byte
	if err := pool.QueryRow(ctx, `
		SELECT config_json
		FROM tool_provider_configs
		WHERE org_id = $1 AND provider_name = 'web_search.tavily'
	`, org.ID).Scan(&rawCfg); err != nil {
		t.Fatalf("load config_json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		t.Fatalf("unmarshal config_json: %v (%s)", err, string(rawCfg))
	}
	if v, ok := cfg["keep"]; !ok || v != true {
		t.Fatalf("unexpected config_json after credential upsert: %v", cfg)
	}

	listAfterKey := doJSON(handler, nethttp.MethodGet, "/v1/tool-providers", nil, authHeader(token))
	if listAfterKey.Code != nethttp.StatusOK {
		t.Fatalf("list after key: %d %s", listAfterKey.Code, listAfterKey.Body.String())
	}
	afterKey := decodeJSONBody[toolProvidersListResponse](t, listAfterKey.Body.Bytes())

	var tavilyPrefix *string
	for _, g := range afterKey.Groups {
		for _, p := range g.Providers {
			if p.ProviderName == "web_search.tavily" {
				tavilyPrefix = p.KeyPrefix
			}
		}
	}
	if tavilyPrefix == nil || *tavilyPrefix != "tvly-123" {
		t.Fatalf("unexpected key prefix: %v", tavilyPrefix)
	}

	// 清除凭证会同时停用
	clearResp := doJSON(handler, nethttp.MethodDelete, "/v1/tool-providers/web_search/web_search.tavily/credential", nil, authHeader(token))
	if clearResp.Code != nethttp.StatusNoContent {
		t.Fatalf("clear credential: %d %s", clearResp.Code, clearResp.Body.String())
	}

	listAfterClear := doJSON(handler, nethttp.MethodGet, "/v1/tool-providers", nil, authHeader(token))
	if listAfterClear.Code != nethttp.StatusOK {
		t.Fatalf("list after clear: %d %s", listAfterClear.Code, listAfterClear.Body.String())
	}
	afterClear := decodeJSONBody[toolProvidersListResponse](t, listAfterClear.Body.Bytes())

	var tavily toolProviderListItem
	found := false
	for _, g := range afterClear.Groups {
		for _, p := range g.Providers {
			if p.ProviderName == "web_search.tavily" {
				tavily = p
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected tavily provider in list")
	}
	if tavily.IsActive {
		t.Fatal("expected tavily inactive after clear")
	}
	if tavily.KeyPrefix != nil {
		t.Fatalf("expected key_prefix cleared, got %v", *tavily.KeyPrefix)
	}
}
