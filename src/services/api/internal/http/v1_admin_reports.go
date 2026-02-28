package http

import (
	"strconv"
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"
)

type adminReportItem struct {
	ID            string   `json:"id"`
	ThreadID      string   `json:"thread_id"`
	ReporterID    string   `json:"reporter_id"`
	ReporterEmail string   `json:"reporter_email"`
	Categories    []string `json:"categories"`
	Feedback      *string  `json:"feedback"`
	CreatedAt     string   `json:"created_at"`
}

type adminReportsResponse struct {
	Data  []adminReportItem `json:"data"`
	Total int               `json:"total"`
}

func adminReportsEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	reportRepo *data.ThreadReportRepository,
	apiKeysRepo *data.APIKeysRepository,
) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

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
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		if r.Method != nethttp.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}

		limit, ok := parseLimit(w, traceID, r.URL.Query().Get("limit"))
		if !ok {
			return
		}

		offset := 0
		if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 0 {
				WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid offset", traceID, nil)
				return
			}
			offset = parsed
		}

		rows, total, err := reportRepo.List(r.Context(), limit, offset)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}

		items := make([]adminReportItem, 0, len(rows))
		for _, row := range rows {
			items = append(items, adminReportItem{
				ID:            row.ID.String(),
				ThreadID:      row.ThreadID.String(),
				ReporterID:    row.ReporterID.String(),
				ReporterEmail: row.ReporterEmail,
				Categories:    row.Categories,
				Feedback:      row.Feedback,
				CreatedAt:     row.CreatedAt.UTC().Format(time.RFC3339Nano),
			})
		}

		writeJSON(w, traceID, nethttp.StatusOK, adminReportsResponse{
			Data:  items,
			Total: total,
		})
	}
}
