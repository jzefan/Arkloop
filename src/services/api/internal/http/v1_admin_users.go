package http

import (
	nethttp "net/http"
	"strings"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type adminUserResponse struct {
	ID              string  `json:"id"`
	DisplayName     string  `json:"display_name"`
	Email           *string `json:"email"`
	EmailVerifiedAt *string `json:"email_verified_at,omitempty"`
	Status          string  `json:"status"`
	AvatarURL       *string `json:"avatar_url,omitempty"`
	Locale          *string `json:"locale,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	LastLoginAt     *string `json:"last_login_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type adminUserDetailResponse struct {
	adminUserResponse
	Orgs []adminUserOrgResponse `json:"orgs"`
}

type adminUserOrgResponse struct {
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
}

func toAdminUserResponse(u data.User) adminUserResponse {
	resp := adminUserResponse{
		ID:          u.ID.String(),
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Status:      u.Status,
		AvatarURL:   u.AvatarURL,
		Locale:      u.Locale,
		Timezone:    u.Timezone,
		CreatedAt:   u.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
	}
	if u.EmailVerifiedAt != nil {
		s := u.EmailVerifiedAt.UTC().Format("2006-01-02T15:04:05.999999999Z")
		resp.EmailVerifiedAt = &s
	}
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.UTC().Format("2006-01-02T15:04:05.999999999Z")
		resp.LastLoginAt = &s
	}
	return resp
}

func adminUsersEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	list := listAdminUsers(authService, membershipRepo, usersRepo, apiKeysRepo)
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.Method {
		case nethttp.MethodGet:
			list(w, r)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func adminUserEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	get := getAdminUser(authService, membershipRepo, usersRepo, apiKeysRepo)
	patch := patchAdminUser(authService, membershipRepo, usersRepo, apiKeysRepo, auditWriter)
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		tail := strings.TrimPrefix(r.URL.Path, "/v1/admin/users/")
		if tail == "" || strings.Contains(tail, "/") {
			WriteError(w, nethttp.StatusNotFound, "http.not_found", "not found", traceID, nil)
			return
		}

		userID, err := uuid.Parse(tail)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid user_id", traceID, nil)
			return
		}

		switch r.Method {
		case nethttp.MethodGet:
			get(w, r, userID)
		case nethttp.MethodPatch:
			patch(w, r, userID)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func listAdminUsers(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}

		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		limit, ok := parseLimit(w, traceID, r.URL.Query().Get("limit"))
		if !ok {
			return
		}

		beforeCreatedAt, beforeID, ok := parseThreadCursor(w, traceID, r.URL.Query())
		if !ok {
			return
		}

		query := strings.TrimSpace(r.URL.Query().Get("q"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		if status != "" && status != "active" && status != "suspended" {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "status must be 'active' or 'suspended'", traceID, nil)
			return
		}

		users, err := usersRepo.List(r.Context(), limit, beforeCreatedAt, beforeID, query, status)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}

		resp := make([]adminUserResponse, 0, len(users))
		for _, u := range users {
			resp = append(resp, toAdminUserResponse(u))
		}
		writeJSON(w, traceID, nethttp.StatusOK, resp)
	}
}

func getAdminUser(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request, uuid.UUID) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request, userID uuid.UUID) {
		traceID := observability.TraceIDFromContext(r.Context())

		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}

		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		user, err := usersRepo.GetByID(r.Context(), userID)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if user == nil {
			WriteError(w, nethttp.StatusNotFound, "users.not_found", "user not found", traceID, nil)
			return
		}

		detail := adminUserDetailResponse{
			adminUserResponse: toAdminUserResponse(*user),
			Orgs:              []adminUserOrgResponse{},
		}

		if membershipRepo != nil {
			membership, err := membershipRepo.GetDefaultForUser(r.Context(), userID)
			if err == nil && membership != nil {
				detail.Orgs = append(detail.Orgs, adminUserOrgResponse{
					OrgID: membership.OrgID.String(),
					Role:  membership.Role,
				})
			}
		}

		writeJSON(w, traceID, nethttp.StatusOK, detail)
	}
}

func patchAdminUser(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
) func(nethttp.ResponseWriter, *nethttp.Request, uuid.UUID) {
	type patchBody struct {
		Status *string `json:"status"`
	}

	return func(w nethttp.ResponseWriter, r *nethttp.Request, userID uuid.UUID) {
		traceID := observability.TraceIDFromContext(r.Context())

		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}

		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		var body patchBody
		if err := decodeJSON(r, &body); err != nil {
			WriteError(w, nethttp.StatusBadRequest, "validation.error", "invalid request body", traceID, nil)
			return
		}

		if body.Status == nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "status is required", traceID, nil)
			return
		}

		newStatus := *body.Status
		if newStatus != "active" && newStatus != "suspended" {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "status must be 'active' or 'suspended'", traceID, nil)
			return
		}

		// 获取更新前的状态用于审计
		existing, err := usersRepo.GetByID(r.Context(), userID)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if existing == nil {
			WriteError(w, nethttp.StatusNotFound, "users.not_found", "user not found", traceID, nil)
			return
		}

		oldStatus := existing.Status
		if oldStatus == newStatus {
			writeJSON(w, traceID, nethttp.StatusOK, toAdminUserResponse(*existing))
			return
		}

		updated, err := usersRepo.UpdateStatus(r.Context(), userID, newStatus)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if updated == nil {
			WriteError(w, nethttp.StatusNotFound, "users.not_found", "user not found", traceID, nil)
			return
		}

		if auditWriter != nil {
			auditWriter.WriteUserStatusChanged(r.Context(), traceID, actor.UserID, userID, oldStatus, newStatus)
		}

		writeJSON(w, traceID, nethttp.StatusOK, toAdminUserResponse(*updated))
	}
}
