package adminapi

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	httpkit "arkloop/services/api/internal/http/httpkit"

	"github.com/google/uuid"
)

var uuidPrefixRegex = regexp.MustCompile(`^[0-9a-fA-F-]{1,36}$`)

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

func calcCacheHitRate(inputTokens, cacheRead, cacheCreation, cachedTokens *int64) *float64 {
	hasAnthropic := (cacheRead != nil && *cacheRead > 0) || (cacheCreation != nil && *cacheCreation > 0)
	hasOpenAI := cachedTokens != nil && *cachedTokens > 0

	if hasAnthropic && hasOpenAI {
		return nil
	}
	if hasAnthropic {
		total := 0.0
		if inputTokens != nil {
			total += float64(*inputTokens)
		}
		if cacheRead != nil {
			total += float64(*cacheRead)
		}
		if cacheCreation != nil {
			total += float64(*cacheCreation)
		}
		if total <= 0 {
			return nil
		}
		read := 0.0
		if cacheRead != nil {
			read = float64(*cacheRead)
		}
		r := read / total
		return &r
	}
	if hasOpenAI && inputTokens != nil && *inputTokens > 0 {
		r := float64(*cachedTokens) / float64(*inputTokens)
		return &r
	}
	return nil
}
