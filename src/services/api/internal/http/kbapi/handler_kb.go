package kbapi

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strings"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

// handlerCtx is the per-request collaborator bundle. Production wiring passes
// real repos; tests pass fakes. Interfaces are intentionally narrow.
type handlerCtx struct {
	kbStore        kbStore
	docStore       docStore
	chunksRepo     chunksReader
	membership     membershipChecker
	blob           blobWriter
	jobs           jobEnqueue
	embedder       embeddingForSearch
	maxUploadBytes int64
}

type kbStore interface {
	Create(ctx context.Context, in data.KBCreate) (*data.KnowledgeBase, error)
	GetByID(ctx context.Context, id uuid.UUID) (*data.KnowledgeBase, error)
	ListByWorkspace(ctx context.Context, accountID uuid.UUID, ws string) ([]data.KnowledgeBase, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type docStore interface {
	Create(ctx context.Context, in data.DocCreate) (*data.KBDocument, error)
	GetByID(ctx context.Context, id uuid.UUID) (*data.KBDocument, error)
	ListByKB(ctx context.Context, kbID uuid.UUID) ([]data.KBDocument, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type blobWriter interface {
	PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error
}

type jobEnqueue interface {
	EnqueueKBIngest(ctx context.Context, accountID, kbID, docID uuid.UUID, workspaceRef, blobSHA256, mimeType, filename, traceID string) (uuid.UUID, error)
}

type chunksReader interface {
	Search(ctx context.Context, kbID uuid.UUID, query []float32, k int) ([]data.KBChunkHit, error)
}

type embeddingForSearch interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// actor is the resolved caller identity. Real routes wrap httpkit.ResolveActor;
// tests use injectActor to set this directly.
type actor struct {
	AccountID uuid.UUID
	UserID    uuid.UUID
}

type actorKey struct{}

func injectActor(r *nethttp.Request, accountID, userID uuid.UUID) *nethttp.Request {
	return r.WithContext(context.WithValue(r.Context(), actorKey{}, actor{AccountID: accountID, UserID: userID}))
}

func actorFromCtx(ctx context.Context) (actor, bool) {
	a, ok := ctx.Value(actorKey{}).(actor)
	return a, ok
}

func writeJSON(w nethttp.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w nethttp.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "message": msg})
}

type createKBReq struct {
	Name         string `json:"name"`
	WorkspaceRef string `json:"workspace_ref"`
	Description  string `json:"description"`
}

func handleCreateKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		var req createKBReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, nethttp.StatusBadRequest, "kb.invalid_json", "invalid json body")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.WorkspaceRef = strings.TrimSpace(req.WorkspaceRef)
		if req.Name == "" || req.WorkspaceRef == "" {
			writeErr(w, nethttp.StatusBadRequest, "kb.missing_field", "name and workspace_ref are required")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, req.WorkspaceRef); err != nil {
			writeErr(w, nethttp.StatusForbidden, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		kb, err := h.kbStore.Create(r.Context(), data.KBCreate{
			AccountID:    a.AccountID,
			WorkspaceRef: req.WorkspaceRef,
			Name:         req.Name,
			Description:  req.Description,
			CreatedBy:    &a.UserID,
		})
		if err != nil {
			if errors.Is(err, data.ErrKBDuplicateName) {
				writeErr(w, nethttp.StatusConflict, "kb.duplicate_name", "kb with this name already exists in this workspace")
				return
			}
			writeErr(w, nethttp.StatusInternalServerError, "internal.error", "failed to create kb")
			return
		}
		writeJSON(w, nethttp.StatusCreated, kbResponse(kb))
	}
}

func handleListKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		ws := strings.TrimSpace(r.URL.Query().Get("workspace_ref"))
		if ws == "" {
			writeErr(w, nethttp.StatusBadRequest, "kb.missing_workspace_ref", "workspace_ref query param is required")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, ws); err != nil {
			writeErr(w, nethttp.StatusForbidden, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		kbs, err := h.kbStore.ListByWorkspace(r.Context(), a.AccountID, ws)
		if err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.error", "list failed")
			return
		}
		items := make([]map[string]any, 0, len(kbs))
		for i := range kbs {
			items = append(items, kbResponse(&kbs[i]))
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"items": items})
	}
}

func handleGetKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		writeJSON(w, nethttp.StatusOK, kbResponse(kb))
	}
}

func handleDeleteKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		if err := h.kbStore.Delete(r.Context(), kb.ID); err != nil {
			if errors.Is(err, data.ErrKBNotFound) {
				writeErr(w, nethttp.StatusNotFound, "kb.not_found", "kb not found")
				return
			}
			writeErr(w, nethttp.StatusInternalServerError, "internal.error", "delete failed")
			return
		}
		w.WriteHeader(nethttp.StatusNoContent)
	}
}

func loadAuthorizedKB(w nethttp.ResponseWriter, r *nethttp.Request, h *handlerCtx, a actor) (*data.KnowledgeBase, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, nethttp.StatusBadRequest, "kb.invalid_id", "invalid kb id")
		return nil, false
	}
	kb, err := h.kbStore.GetByID(r.Context(), id)
	if err != nil || kb == nil || kb.AccountID != a.AccountID {
		writeErr(w, nethttp.StatusNotFound, "kb.not_found", "kb not found")
		return nil, false
	}
	if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
		writeErr(w, nethttp.StatusForbidden, "auth.no_workspace_access", "no access to this workspace")
		return nil, false
	}
	return kb, true
}

func kbResponse(kb *data.KnowledgeBase) map[string]any {
	return map[string]any{
		"id":               kb.ID,
		"name":             kb.Name,
		"workspace_ref":    kb.WorkspaceRef,
		"description":      kb.Description,
		"integration_mode": kb.IntegrationMode,
		"document_count":   kb.DocumentCount,
		"created_at":       kb.CreatedAt,
		"updated_at":       kb.UpdatedAt,
	}
}
