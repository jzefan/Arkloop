package http

import (
	nethttp "net/http"
	"strings"
	"time"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"
)

type createFeedbackRequest struct {
	Feedback string `json:"feedback"`
}

func meFeedback(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	reportRepo *data.ThreadReportRepository,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		if r.Method != nethttp.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}
		if reportRepo == nil {
			WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
			return
		}

		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}

		var body createFeedbackRequest
		if err := decodeJSON(r, &body); err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
			return
		}

		feedback := strings.TrimSpace(body.Feedback)
		if feedback == "" {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "feedback must not be empty", traceID, nil)
			return
		}
		if len(feedback) > 2000 {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "feedback too long", traceID, nil)
			return
		}

		report, err := reportRepo.CreateSuggestion(r.Context(), actor.UserID, feedback)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}

		writeJSON(w, traceID, nethttp.StatusCreated, reportResponse{
			ID:        report.ID.String(),
			CreatedAt: report.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
}
