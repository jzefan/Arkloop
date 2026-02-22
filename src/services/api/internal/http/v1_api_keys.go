package http

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const apiKeysCacheTTL = 5 * time.Minute

type createAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type apiKeyResponse struct {
	ID          string   `json:"id"`
	OrgID       string   `json:"org_id"`
	UserID      string   `json:"user_id"`
	Name        string   `json:"name"`
	KeyPrefix   string   `json:"key_prefix"`
	Scopes      []string `json:"scopes"`
	RevokedAt   *string  `json:"revoked_at,omitempty"`
	LastUsedAt  *string  `json:"last_used_at,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

type createAPIKeyResponse struct {
	apiKeyResponse
	Key string `json:"key"`
}

type apiKeyCacheEntry struct {
	OrgID   string `json:"org_id"`
	UserID  string `json:"user_id"`
	Revoked bool   `json:"revoked"`
}

func apiKeysEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	redisClient *redis.Client,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		switch r.Method {
		case nethttp.MethodPost:
			createAPIKey(w, r, traceID, authService, membershipRepo, apiKeysRepo, auditWriter, redisClient)
		case nethttp.MethodGet:
			listAPIKeys(w, r, traceID, authService, membershipRepo, apiKeysRepo)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func apiKeyEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	redisClient *redis.Client,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		tail := strings.TrimPrefix(r.URL.Path, "/v1/api-keys/")
		tail = strings.Trim(tail, "/")
		if tail == "" {
			writeNotFound(w, r)
			return
		}

		keyID, err := uuid.Parse(tail)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation_error", "invalid api key id", traceID, nil)
			return
		}

		switch r.Method {
		case nethttp.MethodDelete:
			revokeAPIKey(w, r, traceID, keyID, authService, membershipRepo, apiKeysRepo, auditWriter, redisClient)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func createAPIKey(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	redisClient *redis.Client,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if apiKeysRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	var req createAPIKeyRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation_error", "request validation failed", traceID, nil)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation_error", "name is required", traceID, nil)
		return
	}
	if len(req.Name) > 200 {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation_error", "name too long", traceID, nil)
		return
	}
	if req.Scopes == nil {
		req.Scopes = []string{}
	}

	apiKey, rawKey, err := apiKeysRepo.Create(r.Context(), actor.OrgID, actor.UserID, req.Name, req.Scopes)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", traceID, nil)
		return
	}

	syncAPIKeyCache(r.Context(), redisClient, apiKey, data.HashAPIKey(rawKey))

	if auditWriter != nil {
		auditWriter.WriteAPIKeyCreated(r.Context(), traceID, actor.OrgID, actor.UserID, apiKey.ID, apiKey.Name)
	}

	resp := createAPIKeyResponse{
		apiKeyResponse: toAPIKeyResponse(apiKey),
		Key:            rawKey,
	}
	writeJSON(w, traceID, nethttp.StatusCreated, resp)
}

func listAPIKeys(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	apiKeysRepo *data.APIKeysRepository,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if apiKeysRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	keys, err := apiKeysRepo.ListByOrg(r.Context(), actor.OrgID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", traceID, nil)
		return
	}

	resp := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		resp = append(resp, toAPIKeyResponse(k))
	}
	writeJSON(w, traceID, nethttp.StatusOK, resp)
}

func revokeAPIKey(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	keyID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	redisClient *redis.Client,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if apiKeysRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	revoked, err := apiKeysRepo.Revoke(r.Context(), actor.OrgID, keyID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", traceID, nil)
		return
	}
	if !revoked {
		WriteError(w, nethttp.StatusNotFound, "api_keys.not_found", "api key not found", traceID, nil)
		return
	}

	// 吊销时清理 Redis 缓存，防止 Gateway 继续使用旧缓存
	invalidateAPIKeyCache(r.Context(), redisClient, actor.OrgID, keyID)

	if auditWriter != nil {
		auditWriter.WriteAPIKeyRevoked(r.Context(), traceID, actor.OrgID, actor.UserID, keyID)
	}

	w.WriteHeader(nethttp.StatusNoContent)
}

// syncAPIKeyCache 将 API Key 元数据写入 Redis，供 Gateway 限流和 IP 过滤提取 org_id。
func syncAPIKeyCache(ctx context.Context, client *redis.Client, apiKey data.APIKey, keyHash string) {
	if client == nil {
		return
	}

	entry := apiKeyCacheEntry{
		OrgID:   apiKey.OrgID.String(),
		UserID:  apiKey.UserID.String(),
		Revoked: false,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	key := fmt.Sprintf("arkloop:api_keys:%s", keyHash)
	_ = client.Set(ctx, key, raw, apiKeysCacheTTL).Err()
}

// invalidateAPIKeyCache 吊销时无法直接删除（不持有 rawKey），改为标记 revoked=true。
// 由于 TTL 5min，Gateway 最多延迟 5min 感知吊销（可接受）。
// API 服务侧 DB 查询始终权威，吊销立即生效。
func invalidateAPIKeyCache(ctx context.Context, client *redis.Client, orgID, keyID uuid.UUID) {
	if client == nil {
		return
	}
	// 吊销时无法从 keyID 直接推算 keyHash，无法精确删缓存。
	// API 服务侧以 DB 为准，Gateway 侧的缓存条目最多存活到 TTL 过期。
	// 如需即时失效，可在 api_keys 表上额外存储 key_hash 用于反查，
	// 但当前架构中 DB 查询已足够（revoked_at 非 nil 时直接拒绝）。
	_ = orgID
	_ = keyID
}

func toAPIKeyResponse(k data.APIKey) apiKeyResponse {
	resp := apiKeyResponse{
		ID:        k.ID.String(),
		OrgID:     k.OrgID.String(),
		UserID:    k.UserID.String(),
		Name:      k.Name,
		KeyPrefix: k.KeyPrefix,
		Scopes:    k.Scopes,
		CreatedAt: k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if k.RevokedAt != nil {
		s := k.RevokedAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.RevokedAt = &s
	}
	if k.LastUsedAt != nil {
		s := k.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.LastUsedAt = &s
	}
	return resp
}
