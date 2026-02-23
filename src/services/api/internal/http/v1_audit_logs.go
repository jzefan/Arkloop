package http

import (
	"strconv"
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type auditLogResponse struct {
	ID          string         `json:"id"`
	OrgID       *string        `json:"org_id,omitempty"`
	ActorUserID *string        `json:"actor_user_id,omitempty"`
	Action      string         `json:"action"`
	TargetType  *string        `json:"target_type,omitempty"`
	TargetID    *string        `json:"target_id,omitempty"`
	TraceID     string         `json:"trace_id"`
	Metadata    map[string]any `json:"metadata"`
	IPAddress   *string        `json:"ip_address,omitempty"`
	UserAgent   *string        `json:"user_agent,omitempty"`
	CreatedAt   string         `json:"created_at"`

	BeforeState *string `json:"before_state,omitempty"`
	AfterState  *string `json:"after_state,omitempty"`
}

func auditLogsEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	auditLogRepo *data.AuditLogRepository,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if r.Method != nethttp.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}
		listAuditLogs(w, r, authService, membershipRepo, auditLogRepo, apiKeysRepo)
	}
}

func listAuditLogs(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	auditLogRepo *data.AuditLogRepository,
	apiKeysRepo *data.APIKeysRepository,
) {
	traceID := observability.TraceIDFromContext(r.Context())

	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if auditLogRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
	if !ok {
		return
	}

	isPlatformAdmin := actor.HasPermission(auth.PermPlatformAdmin)
	if !isPlatformAdmin {
		if !requirePerm(actor, auth.PermOrgAuditRead, w, traceID) {
			return
		}
	}

	q := r.URL.Query()
	params := data.AuditLogListParams{}

	// org_id: 非平台管理员必须提供且只能查自己 org
	if rawOrg := q.Get("org_id"); rawOrg != "" {
		parsed, err := uuid.Parse(rawOrg)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid org_id", traceID, nil)
			return
		}
		if !isPlatformAdmin && parsed != actor.OrgID {
			WriteError(w, nethttp.StatusForbidden, "auth.forbidden", "access denied", traceID, nil)
			return
		}
		params.OrgID = &parsed
	} else if !isPlatformAdmin {
		params.OrgID = &actor.OrgID
	}

	if v := q.Get("action"); v != "" {
		v = strings.TrimSpace(v)
		params.Action = &v
	}
	if v := q.Get("actor_user_id"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid actor_user_id", traceID, nil)
			return
		}
		params.ActorUserID = &parsed
	}
	if v := q.Get("target_type"); v != "" {
		v = strings.TrimSpace(v)
		params.TargetType = &v
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid since: must be RFC3339", traceID, nil)
			return
		}
		params.Since = &t
	}
	if v := q.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid until: must be RFC3339", traceID, nil)
			return
		}
		params.Until = &t
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "limit must be 1-200", traceID, nil)
			return
		}
		params.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "offset must be >= 0", traceID, nil)
			return
		}
		params.Offset = n
	}
	if q.Get("include") == "state" {
		params.IncludeState = true
	}

	logs, total, err := auditLogRepo.List(r.Context(), params)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	resp := make([]auditLogResponse, 0, len(logs))
	for _, l := range logs {
		resp = append(resp, toAuditLogResponse(l))
	}

	writeJSON(w, traceID, nethttp.StatusOK, map[string]any{
		"data":  resp,
		"total": total,
	})
}

func toAuditLogResponse(l data.AuditLog) auditLogResponse {
	r := auditLogResponse{
		ID:        l.ID.String(),
		Action:    l.Action,
		TraceID:   l.TraceID,
		Metadata:  l.MetadataJSON,
		CreatedAt: l.CreatedAt.UTC().Format(time.RFC3339),
		TargetType: l.TargetType,
		TargetID:   l.TargetID,
		IPAddress:  l.IPAddress,
		UserAgent:  l.UserAgent,
	}
	if l.MetadataJSON == nil {
		r.Metadata = map[string]any{}
	}
	if l.OrgID != nil {
		s := l.OrgID.String()
		r.OrgID = &s
	}
	if l.ActorUserID != nil {
		s := l.ActorUserID.String()
		r.ActorUserID = &s
	}
	r.BeforeState = l.BeforeStateJSON
	r.AfterState = l.AfterStateJSON
	return r
}
