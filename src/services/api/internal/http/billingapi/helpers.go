package billingapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	httpkit "arkloop/services/api/internal/http/httpkit"

	"github.com/google/uuid"
)

func parseLimit(w http.ResponseWriter, traceID string, raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return 50, true
	}

	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 1 || parsed > 200 {
		httpkit.WriteError(w, http.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return 0, false
	}
	return parsed, true
}

func parseThreadCursor(
	w http.ResponseWriter,
	traceID string,
	values url.Values,
) (*time.Time, *uuid.UUID, bool) {
	beforeCreatedAtRaw := strings.TrimSpace(first(values, "before_created_at"))
	beforeIDRaw := strings.TrimSpace(first(values, "before_id"))

	if (beforeCreatedAtRaw == "") != (beforeIDRaw == "") {
		httpkit.WriteError(
			w,
			http.StatusUnprocessableEntity,
			"validation.error",
			"request validation failed",
			traceID,
			map[string]any{"reason": "cursor_incomplete", "required": []string{"before_created_at", "before_id"}},
		)
		return nil, nil, false
	}
	if beforeCreatedAtRaw == "" {
		return nil, nil, true
	}

	parsedTime, err := time.Parse(time.RFC3339Nano, beforeCreatedAtRaw)
	if err != nil {
		httpkit.WriteError(w, http.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return nil, nil, false
	}
	parsedID, err := uuid.Parse(beforeIDRaw)
	if err != nil {
		httpkit.WriteError(w, http.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return nil, nil, false
	}

	return &parsedTime, &parsedID, true
}

func first(values url.Values, key string) string {
	raw := values[key]
	if len(raw) == 0 {
		return ""
	}
	return raw[0]
}
