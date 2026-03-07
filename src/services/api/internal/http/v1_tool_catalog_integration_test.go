package http

import (
	"context"
	"io"
	nethttp "net/http"
	"testing"
	"time"

	"arkloop/services/api/internal/auth"
	apiCrypto "arkloop/services/api/internal/crypto"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"
	sharedtoolmeta "arkloop/services/shared/toolmeta"
)

func TestToolCatalogSupportsPlatformAndOrgOverrides(t *testing.T) {
	db := setupTestDatabase(t, "api_go_tool_catalog")

	ctx := context.Background()
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 32, MinConns: 0})
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
	overridesRepo, err := data.NewToolDescriptionOverridesRepository(pool)
	if err != nil {
		t.Fatalf("tool description repo: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 5)
	}
	ring, err := apiCrypto.NewKeyRing(map[int][]byte{1: key})
	if err != nil {
		t.Fatalf("new key ring: %v", err)
	}
	passwordHasher, err := auth.NewBcryptPasswordHasher(0)
	if err != nil {
		t.Fatalf("new password hasher: %v", err)
	}
	_ = ring
	tokenService, err := auth.NewJwtAccessTokenService("test-secret-should-be-long-enough-32chars", 3600, 2592000)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	authService, err := auth.NewService(userRepo, credRepo, membershipRepo, passwordHasher, tokenService, refreshTokenRepo, nil)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	org, err := orgRepo.Create(ctx, "tool-catalog-org", "Tool Catalog Org", "personal")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	platformAdmin, err := userRepo.Create(ctx, "tool-admin", "tool-admin@test.com", "en")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := membershipRepo.Create(ctx, org.ID, platformAdmin.ID, auth.RolePlatformAdmin); err != nil {
		t.Fatalf("create admin membership: %v", err)
	}
	adminToken, err := tokenService.Issue(platformAdmin.ID, org.ID, auth.RolePlatformAdmin, time.Now().UTC())
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}

	handler := NewHandler(HandlerConfig{
		Pool:                         pool,
		DirectPool:                   pool,
		Logger:                       logger,
		AuthService:                  authService,
		OrgMembershipRepo:            membershipRepo,
		ToolDescriptionOverridesRepo: overridesRepo,
	})

	listResp := doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog", nil, authHeader(adminToken))
	if listResp.Code != nethttp.StatusOK {
		t.Fatalf("list: %d %s", listResp.Code, listResp.Body.String())
	}
	catalog := decodeJSONBody[toolCatalogResponse](t, listResp.Body.Bytes())
	for _, groupName := range []string{"web_search", "web_fetch", "sandbox", "memory", "browser", "document", "orchestration", "internal"} {
		if _, ok := findCatalogGroup(catalog, groupName); !ok {
			t.Fatalf("missing group %s", groupName)
		}
	}

	webSearch, ok := findCatalogTool(catalog, "web_search", "web_search")
	if !ok {
		t.Fatal("web_search tool missing")
	}
	if webSearch.Label != "Web search" {
		t.Fatalf("unexpected label: %s", webSearch.Label)
	}
	if webSearch.DescriptionSource != toolDescriptionSourceDefault {
		t.Fatalf("expected default source, got %s", webSearch.DescriptionSource)
	}
	if webSearch.HasOverride {
		t.Fatal("default tool should not be marked overridden")
	}
	if webSearch.LLMDescription != sharedtoolmeta.Must("web_search").LLMDescription {
		t.Fatal("unexpected default llm description")
	}

	platformOverride := map[string]any{"description": "platform override for web search"}
	putPlatform := doJSON(handler, nethttp.MethodPut, "/v1/tool-catalog/web_search/description", platformOverride, authHeader(adminToken))
	if putPlatform.Code != nethttp.StatusNoContent {
		t.Fatalf("put platform override: %d %s", putPlatform.Code, putPlatform.Body.String())
	}

	listPlatform := doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog", nil, authHeader(adminToken))
	platformCatalog := decodeJSONBody[toolCatalogResponse](t, listPlatform.Body.Bytes())
	webSearch, _ = findCatalogTool(platformCatalog, "web_search", "web_search")
	if webSearch.LLMDescription != "platform override for web search" {
		t.Fatalf("unexpected platform description: %s", webSearch.LLMDescription)
	}
	if !webSearch.HasOverride {
		t.Fatal("platform override should set has_override")
	}
	if webSearch.DescriptionSource != toolDescriptionSourcePlatform {
		t.Fatalf("expected platform source, got %s", webSearch.DescriptionSource)
	}

	listOrg := doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog?scope=org", nil, authHeader(adminToken))
	if listOrg.Code != nethttp.StatusOK {
		t.Fatalf("list org: %d %s", listOrg.Code, listOrg.Body.String())
	}
	orgCatalog := decodeJSONBody[toolCatalogResponse](t, listOrg.Body.Bytes())
	webSearch, _ = findCatalogTool(orgCatalog, "web_search", "web_search")
	if webSearch.LLMDescription != "platform override for web search" {
		t.Fatalf("org view should inherit platform override, got %s", webSearch.LLMDescription)
	}
	if webSearch.HasOverride {
		t.Fatal("org inherited description should not set has_override")
	}
	if webSearch.DescriptionSource != toolDescriptionSourcePlatform {
		t.Fatalf("expected platform source in org scope, got %s", webSearch.DescriptionSource)
	}

	orgOverride := map[string]any{"description": "org override for web search"}
	putOrg := doJSON(handler, nethttp.MethodPut, "/v1/tool-catalog/web_search/description?scope=org", orgOverride, authHeader(adminToken))
	if putOrg.Code != nethttp.StatusNoContent {
		t.Fatalf("put org override: %d %s", putOrg.Code, putOrg.Body.String())
	}

	listOrg = doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog?scope=org", nil, authHeader(adminToken))
	orgCatalog = decodeJSONBody[toolCatalogResponse](t, listOrg.Body.Bytes())
	webSearch, _ = findCatalogTool(orgCatalog, "web_search", "web_search")
	if webSearch.LLMDescription != "org override for web search" {
		t.Fatalf("unexpected org description: %s", webSearch.LLMDescription)
	}
	if !webSearch.HasOverride {
		t.Fatal("org override should set has_override")
	}
	if webSearch.DescriptionSource != toolDescriptionSourceOrg {
		t.Fatalf("expected org source, got %s", webSearch.DescriptionSource)
	}

	deleteOrg := doJSON(handler, nethttp.MethodDelete, "/v1/tool-catalog/web_search/description?scope=org", nil, authHeader(adminToken))
	if deleteOrg.Code != nethttp.StatusNoContent {
		t.Fatalf("delete org override: %d %s", deleteOrg.Code, deleteOrg.Body.String())
	}

	listOrg = doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog?scope=org", nil, authHeader(adminToken))
	orgCatalog = decodeJSONBody[toolCatalogResponse](t, listOrg.Body.Bytes())
	webSearch, _ = findCatalogTool(orgCatalog, "web_search", "web_search")
	if webSearch.LLMDescription != "platform override for web search" {
		t.Fatalf("org reset should fall back to platform, got %s", webSearch.LLMDescription)
	}
	if webSearch.HasOverride {
		t.Fatal("org reset should clear has_override")
	}
	if webSearch.DescriptionSource != toolDescriptionSourcePlatform {
		t.Fatalf("expected platform source after org reset, got %s", webSearch.DescriptionSource)
	}

	unknown := doJSON(handler, nethttp.MethodPut, "/v1/tool-catalog/not_real/description", platformOverride, authHeader(adminToken))
	if unknown.Code != nethttp.StatusNotFound {
		t.Fatalf("unknown tool should be 404, got %d", unknown.Code)
	}
}

func TestToolCatalogScopePermissions(t *testing.T) {
	db := setupTestDatabase(t, "api_go_tool_catalog_perms")

	ctx := context.Background()
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 32, MinConns: 0})
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
	overridesRepo, err := data.NewToolDescriptionOverridesRepository(pool)
	if err != nil {
		t.Fatalf("tool description repo: %v", err)
	}
	passwordHasher, err := auth.NewBcryptPasswordHasher(0)
	if err != nil {
		t.Fatalf("new password hasher: %v", err)
	}
	tokenService, err := auth.NewJwtAccessTokenService("test-secret-should-be-long-enough-32chars", 3600, 2592000)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	authService, err := auth.NewService(userRepo, credRepo, membershipRepo, passwordHasher, tokenService, refreshTokenRepo, nil)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}

	org, err := orgRepo.Create(ctx, "tool-catalog-org-member", "Tool Catalog Org Member", "personal")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	member, err := userRepo.Create(ctx, "tool-member", "tool-member@test.com", "en")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if _, err := membershipRepo.Create(ctx, org.ID, member.ID, auth.RoleOrgMember); err != nil {
		t.Fatalf("create member membership: %v", err)
	}
	memberToken, err := tokenService.Issue(member.ID, org.ID, auth.RoleOrgMember, time.Now().UTC())
	if err != nil {
		t.Fatalf("issue member token: %v", err)
	}

	handler := NewHandler(HandlerConfig{
		Pool:                         pool,
		DirectPool:                   pool,
		Logger:                       logger,
		AuthService:                  authService,
		OrgMembershipRepo:            membershipRepo,
		ToolDescriptionOverridesRepo: overridesRepo,
	})

	platformResp := doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog", nil, authHeader(memberToken))
	if platformResp.Code != nethttp.StatusForbidden {
		t.Fatalf("org member platform scope should be 403, got %d", platformResp.Code)
	}

	orgResp := doJSON(handler, nethttp.MethodGet, "/v1/tool-catalog?scope=org", nil, authHeader(memberToken))
	if orgResp.Code != nethttp.StatusForbidden {
		t.Fatalf("org member org scope should be 403, got %d", orgResp.Code)
	}
}

func findCatalogGroup(resp toolCatalogResponse, groupName string) (toolCatalogGroup, bool) {
	for _, group := range resp.Groups {
		if group.Group == groupName {
			return group, true
		}
	}
	return toolCatalogGroup{}, false
}

func findCatalogTool(resp toolCatalogResponse, groupName string, toolName string) (toolCatalogItem, bool) {
	group, ok := findCatalogGroup(resp, groupName)
	if !ok {
		return toolCatalogItem{}, false
	}
	for _, tool := range group.Tools {
		if tool.Name == toolName {
			return tool, true
		}
	}
	return toolCatalogItem{}, false
}
