package http

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const ipRulesCacheTTL = 5 * time.Minute

type createIPRuleRequest struct {
	Type string  `json:"type"`
	CIDR string  `json:"cidr"`
	Note *string `json:"note"`
}

type ipRuleResponse struct {
	ID        string  `json:"id"`
	OrgID     string  `json:"org_id"`
	Type      string  `json:"type"`
	CIDR      string  `json:"cidr"`
	Note      *string `json:"note,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type ipRulesCachePayload struct {
	Allowlist []string `json:"allowlist"`
	Blocklist []string `json:"blocklist"`
}

func ipRulesEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	ipRulesRepo *data.IPRulesRepository,
	redisClient *redis.Client,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		switch r.Method {
		case nethttp.MethodPost:
			createIPRule(w, r, traceID, authService, membershipRepo, ipRulesRepo, redisClient)
		case nethttp.MethodGet:
			listIPRules(w, r, traceID, authService, membershipRepo, ipRulesRepo)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func ipRuleEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	ipRulesRepo *data.IPRulesRepository,
	redisClient *redis.Client,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		tail := strings.TrimPrefix(r.URL.Path, "/v1/ip-rules/")
		tail = strings.Trim(tail, "/")
		if tail == "" {
			writeNotFound(w, r)
			return
		}

		ruleID, err := uuid.Parse(tail)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid rule id", traceID, nil)
			return
		}

		switch r.Method {
		case nethttp.MethodDelete:
			deleteIPRule(w, r, traceID, ruleID, authService, membershipRepo, ipRulesRepo, redisClient)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func createIPRule(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	ipRulesRepo *data.IPRulesRepository,
	redisClient *redis.Client,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if ipRulesRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	var req createIPRuleRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return
	}

	req.Type = strings.TrimSpace(req.Type)
	req.CIDR = strings.TrimSpace(req.CIDR)

	if req.Type == "" || req.CIDR == "" {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "type and cidr are required", traceID, nil)
		return
	}
	if req.Type != string(data.IPRuleAllowlist) && req.Type != string(data.IPRuleBlocklist) {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "type must be allowlist or blocklist", traceID, nil)
		return
	}

	rule, err := ipRulesRepo.Create(r.Context(), actor.OrgID, data.IPRuleType(req.Type), req.CIDR, req.Note)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	syncIPRulesCache(r.Context(), ipRulesRepo, redisClient, actor.OrgID)
	writeJSON(w, traceID, nethttp.StatusCreated, toIPRuleResponse(rule))
}

func listIPRules(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	ipRulesRepo *data.IPRulesRepository,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if ipRulesRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	rules, err := ipRulesRepo.ListByOrg(r.Context(), actor.OrgID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	resp := make([]ipRuleResponse, 0, len(rules))
	for _, rule := range rules {
		resp = append(resp, toIPRuleResponse(rule))
	}

	writeJSON(w, traceID, nethttp.StatusOK, resp)
}

func deleteIPRule(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	ruleID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	ipRulesRepo *data.IPRulesRepository,
	redisClient *redis.Client,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if ipRulesRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	deleted, err := ipRulesRepo.Delete(r.Context(), actor.OrgID, ruleID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}
	if !deleted {
		WriteError(w, nethttp.StatusNotFound, "ip_rules.not_found", "rule not found", traceID, nil)
		return
	}

	syncIPRulesCache(r.Context(), ipRulesRepo, redisClient, actor.OrgID)
	w.WriteHeader(nethttp.StatusNoContent)
}

// syncIPRulesCache 将 org 的规则集写入 Redis，失败时只记录日志（不阻断请求）。
func syncIPRulesCache(ctx context.Context, repo *data.IPRulesRepository, client *redis.Client, orgID uuid.UUID) {
	if client == nil {
		return
	}

	rules, err := repo.ListByOrg(ctx, orgID)
	if err != nil {
		return
	}

	payload := ipRulesCachePayload{
		Allowlist: []string{},
		Blocklist: []string{},
	}
	for _, rule := range rules {
		switch rule.Type {
		case data.IPRuleAllowlist:
			payload.Allowlist = append(payload.Allowlist, rule.CIDR)
		case data.IPRuleBlocklist:
			payload.Blocklist = append(payload.Blocklist, rule.CIDR)
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}

	key := fmt.Sprintf("arkloop:ip_rules:%s", orgID.String())
	_ = client.Set(ctx, key, raw, ipRulesCacheTTL).Err()
}

func toIPRuleResponse(rule data.IPRule) ipRuleResponse {
	return ipRuleResponse{
		ID:        rule.ID.String(),
		OrgID:     rule.OrgID.String(),
		Type:      string(rule.Type),
		CIDR:      rule.CIDR,
		Note:      rule.Note,
		CreatedAt: rule.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
