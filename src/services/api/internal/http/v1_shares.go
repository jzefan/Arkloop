package http

import (
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type createShareRequest struct {
	AccessType string  `json:"access_type"` // "public" | "password"
	Password   *string `json:"password"`
	LiveUpdate *bool   `json:"live_update"`
	MessageID  *string `json:"message_id"`
}

type shareResponse struct {
	ID                string  `json:"id"`
	Token             string  `json:"token"`
	URL               string  `json:"url"`
	AccessType        string  `json:"access_type"`
	Password          *string `json:"password,omitempty"`
	LiveUpdate        bool    `json:"live_update"`
	SnapshotTurnCount int     `json:"snapshot_turn_count"`
	MessageID         *string `json:"message_id,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

func handleThreadShare(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	actor *actor,
	thread *data.Thread,
	threadShareRepo *data.ThreadShareRepository,
	messageRepo *data.MessageRepository,
) {
	switch r.Method {
	case nethttp.MethodPost:
		createShare(w, r, traceID, actor, thread, threadShareRepo, messageRepo)
	case nethttp.MethodGet:
		listShares(w, r, traceID, actor, thread, threadShareRepo)
	case nethttp.MethodDelete:
		deleteShare(w, r, traceID, actor, thread, threadShareRepo)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func createShare(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	actor *actor,
	thread *data.Thread,
	threadShareRepo *data.ThreadShareRepository,
	messageRepo *data.MessageRepository,
) {
	if threadShareRepo == nil || messageRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	var body createShareRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return
	}

	if body.AccessType == "" {
		body.AccessType = "public"
	}

	// 消息级分享强制 public
	var messageID *uuid.UUID
	if body.MessageID != nil {
		mid, err := uuid.Parse(*body.MessageID)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid message_id", traceID, nil)
			return
		}
		messageID = &mid
		body.AccessType = "public"
	}

	if body.AccessType != "public" && body.AccessType != "password" {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "access_type must be 'public' or 'password'", traceID, nil)
		return
	}
	if body.AccessType == "password" {
		if body.Password == nil || strings.TrimSpace(*body.Password) == "" {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "password is required for password-protected shares", traceID, nil)
			return
		}
		if len(*body.Password) > 128 {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "password too long", traceID, nil)
			return
		}
	}

	messages, err := messageRepo.ListByThread(r.Context(), thread.OrgID, thread.ID, 10000)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	snapshotCount := len(messages)
	turnCount := 0
	for _, msg := range messages {
		if msg.Role == "user" {
			turnCount++
		}
	}

	// 消息级分享：验证消息存在，计算 Q&A 对数量
	if messageID != nil {
		found := false
		for _, msg := range messages {
			if msg.ID == *messageID {
				found = true
				break
			}
		}
		if !found {
			WriteError(w, nethttp.StatusNotFound, "messages.not_found", "message not found in this thread", traceID, nil)
			return
		}
		// Q&A 对最多 2 条（用户问题 + 助手回复）
		snapshotCount = 2
		turnCount = 1
	}

	liveUpdate := false
	if body.LiveUpdate != nil && messageID == nil {
		liveUpdate = *body.LiveUpdate
	}

	token, err := data.GenerateShareToken()
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	var password *string
	if body.AccessType == "password" {
		p := strings.TrimSpace(*body.Password)
		password = &p
	}

	share, err := threadShareRepo.Create(
		r.Context(), thread.ID, token, body.AccessType, password,
		snapshotCount, liveUpdate, turnCount, actor.UserID, messageID,
	)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	resp := shareResponse{
		ID:                share.ID.String(),
		Token:             share.Token,
		URL:               "/s/" + share.Token,
		AccessType:        share.AccessType,
		Password:          share.Password,
		LiveUpdate:        share.LiveUpdate,
		SnapshotTurnCount: share.SnapshotTurnCount,
		CreatedAt:         share.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if share.MessageID != nil {
		mid := share.MessageID.String()
		resp.MessageID = &mid
	}
	writeJSON(w, traceID, nethttp.StatusOK, resp)
}

func listShares(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	actor *actor,
	thread *data.Thread,
	threadShareRepo *data.ThreadShareRepository,
) {
	if threadShareRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	shares, err := threadShareRepo.ListByThreadID(r.Context(), thread.ID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	resp := make([]shareResponse, 0, len(shares))
	for _, s := range shares {
		sr := shareResponse{
			ID:                s.ID.String(),
			Token:             s.Token,
			URL:               "/s/" + s.Token,
			AccessType:        s.AccessType,
			Password:          s.Password,
			LiveUpdate:        s.LiveUpdate,
			SnapshotTurnCount: s.SnapshotTurnCount,
			CreatedAt:         s.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if s.MessageID != nil {
			mid := s.MessageID.String()
			sr.MessageID = &mid
		}
		resp = append(resp, sr)
	}

	writeJSON(w, traceID, nethttp.StatusOK, resp)
}

func deleteShare(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	actor *actor,
	thread *data.Thread,
	threadShareRepo *data.ThreadShareRepository,
) {
	if threadShareRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	shareIDStr := r.URL.Query().Get("id")
	if shareIDStr == "" {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "share id is required", traceID, nil)
		return
	}
	shareID, err := uuid.Parse(shareIDStr)
	if err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid share id", traceID, nil)
		return
	}

	deleted, err := threadShareRepo.DeleteByID(r.Context(), shareID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}
	if !deleted {
		WriteError(w, nethttp.StatusNotFound, "shares.not_found", "no share link exists", traceID, nil)
		return
	}

	w.WriteHeader(nethttp.StatusNoContent)
}

// shareEntry 作为认证端点的入口，在 threadEntry 的 :share action 中调用。
func shareEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	threadRepo *data.ThreadRepository,
	threadShareRepo *data.ThreadShareRepository,
	messageRepo *data.MessageRepository,
	auditWriter *audit.Writer,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request, uuid.UUID) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request, threadID uuid.UUID) {
		traceID := observability.TraceIDFromContext(r.Context())
		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}
		if threadRepo == nil {
			WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
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

		if !authorizeThreadOrAudit(w, r, traceID, actor, "threads.share", thread, auditWriter) {
			return
		}

		handleThreadShare(w, r, traceID, actor, thread, threadShareRepo, messageRepo)
	}
}
