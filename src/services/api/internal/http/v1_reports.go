package http

import (
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type createReportRequest struct {
	Categories []string `json:"categories"`
	Feedback   *string  `json:"feedback"`
}

type reportResponse struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

var validReportCategories = map[string]bool{
	"inaccurate":           true,
	"out_of_date":          true,
	"too_short":            true,
	"too_long":             true,
	"harmful_or_offensive": true,
	"wrong_sources":        true,
}

func reportEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	threadRepo *data.ThreadRepository,
	threadReportRepo *data.ThreadReportRepository,
	auditWriter *audit.Writer,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request, uuid.UUID) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request, threadID uuid.UUID) {
		traceID := observability.TraceIDFromContext(r.Context())
		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}
		if threadRepo == nil || threadReportRepo == nil {
			WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
			return
		}

		if r.Method != nethttp.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}

		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, auditWriter)
		if !ok {
			return
		}
		if !requirePerm(actor, auth.PermDataThreadsWrite, w, traceID) {
			return
		}

		thread, err := threadRepo.GetByID(r.Context(), threadID)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if thread == nil {
			WriteError(w, nethttp.StatusNotFound, "threads.not_found", "thread not found", traceID, nil)
			return
		}

		if !authorizeThreadOrAudit(w, r, traceID, actor, "threads.report", thread, auditWriter) {
			return
		}

		var body createReportRequest
		if err := decodeJSON(r, &body); err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
			return
		}

		if len(body.Categories) == 0 {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "categories must not be empty", traceID, nil)
			return
		}
		for _, cat := range body.Categories {
			if !validReportCategories[cat] {
				WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid category: "+cat, traceID, nil)
				return
			}
		}

		if body.Feedback != nil && len(*body.Feedback) > 2000 {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "feedback too long", traceID, nil)
			return
		}

		report, err := threadReportRepo.Create(r.Context(), thread.ID, actor.UserID, body.Categories, body.Feedback)
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
