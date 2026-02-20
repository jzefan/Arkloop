package http

import (
	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type actor struct {
	OrgID   uuid.UUID
	UserID  uuid.UUID
	OrgRole string
}

func authenticateActor(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
) (*actor, bool) {
	user, ok := authenticateUser(w, r, traceID, authService)
	if !ok {
		return nil, false
	}

	if membershipRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return nil, false
	}

	membership, err := membershipRepo.GetDefaultForUser(r.Context(), user.ID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", traceID, nil)
		return nil, false
	}
	if membership == nil {
		WriteError(w, nethttp.StatusForbidden, "auth.no_org_membership", "user has no org membership", traceID, nil)
		return nil, false
	}

	return &actor{
		OrgID:   membership.OrgID,
		UserID:  user.ID,
		OrgRole: membership.Role,
	}, true
}

func writeNotFound(w nethttp.ResponseWriter, r *nethttp.Request) {
	traceID := observability.TraceIDFromContext(r.Context())
	WriteError(w, nethttp.StatusNotFound, "http_error", "Not Found", traceID, nil)
}
