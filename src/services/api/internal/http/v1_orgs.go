package http

import (
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"
)

type orgResponse struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
}

type createWorkspaceRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func orgsEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	orgRepo *data.OrgRepository,
	orgService *auth.OrgService,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// GET /v1/orgs/me
		if r.Method == nethttp.MethodGet && strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/me") {
			listMyOrgs(w, r, authService, membershipRepo, orgRepo, apiKeysRepo)
			return
		}

		// POST /v1/orgs
		if r.Method == nethttp.MethodPost && (r.URL.Path == "/v1/orgs" || r.URL.Path == "/v1/orgs/") {
			createWorkspace(w, r, authService, membershipRepo, orgService, apiKeysRepo)
			return
		}

		writeNotFound(w, r)
	}
}

func listMyOrgs(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	orgRepo *data.OrgRepository,
	apiKeysRepo *data.APIKeysRepository,
) {
	traceID := observability.TraceIDFromContext(r.Context())
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}

	actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
	if !ok {
		return
	}

	orgs, err := orgRepo.ListByUser(r.Context(), actor.UserID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	resp := make([]orgResponse, 0, len(orgs))
	for _, o := range orgs {
		resp = append(resp, toOrgResponse(o))
	}
	writeJSON(w, traceID, nethttp.StatusOK, resp)
}

func createWorkspace(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	orgService *auth.OrgService,
	apiKeysRepo *data.APIKeysRepository,
) {
	traceID := observability.TraceIDFromContext(r.Context())
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if orgService == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
	if !ok {
		return
	}

	var req createWorkspaceRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.Name = strings.TrimSpace(req.Name)

	if req.Slug == "" || len(req.Slug) > 100 {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "slug must be 1-100 characters", traceID, nil)
		return
	}
	if req.Name == "" || len(req.Name) > 200 {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "name must be 1-200 characters", traceID, nil)
		return
	}

	result, err := orgService.CreateWorkspace(r.Context(), req.Slug, req.Name, actor.UserID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	writeJSON(w, traceID, nethttp.StatusCreated, toOrgResponse(result.Org))
}

func toOrgResponse(o data.Org) orgResponse {
	return orgResponse{
		ID:        o.ID.String(),
		Slug:      o.Slug,
		Name:      o.Name,
		Type:      o.Type,
		CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}
