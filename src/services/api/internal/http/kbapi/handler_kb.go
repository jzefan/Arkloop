package kbapi

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strings"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/http/httpkit"
	"arkloop/services/api/internal/observability"
	sharedenvironmentref "arkloop/services/shared/environmentref"

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

	examIntegrationEnabled bool

	authService           *auth.Service
	accountMembershipRepo *data.AccountMembershipRepository
	apiKeysRepo           *data.APIKeysRepository
	auditWriter           *audit.Writer
	profileRepo           *data.ProfileRegistriesRepository
	workspaceRepo         *data.WorkspaceRegistriesRepository
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
	CountByBlobSHA256(ctx context.Context, sha string) (int, error)
}

type blobWriter interface {
	PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error
	DeleteBlob(ctx context.Context, workspaceRef, sha256 string) error
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

type HandlerCtxExt = handlerCtx

func NewHandlerCtx(deps Deps) *handlerCtx {
	return &handlerCtx{
		kbStore:                deps.KnowledgeBasesRepo,
		docStore:               deps.KBDocumentsRepo,
		chunksRepo:             deps.KBChunksRepo,
		membership:             &WorkspaceMembership{Workspaces: deps.WorkspaceRegistriesRepo},
		blob:                   &WorkspaceBlobAdapter{Store: deps.BlobStore},
		jobs:                   deps.JobEnqueuer,
		embedder:               deps.Embedder,
		maxUploadBytes:         deps.MaxUploadBytes,
		examIntegrationEnabled: deps.ExamIntegrationEnabled,
		authService:            deps.AuthService,
		accountMembershipRepo:  deps.AccountMembershipRepo,
		apiKeysRepo:            deps.APIKeysRepo,
		auditWriter:            deps.AuditWriter,
		profileRepo:            deps.ProfileRegistriesRepo,
		workspaceRepo:          deps.WorkspaceRegistriesRepo,
	}
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
	Name            string  `json:"name"`
	WorkspaceRef    string  `json:"workspace_ref"`
	Description     string  `json:"description"`
	Visibility      string  `json:"visibility"`       // "" -> workspace_member
	IntegrationMode string  `json:"integration_mode"` // "" -> standalone
	ExamCourseID    *string `json:"exam_course_id"`   // required iff mode=exam
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
		if req.Name == "" {
			writeErr(w, nethttp.StatusBadRequest, "kb.missing_field", "name is required")
			return
		}
		// visibility validation
		switch req.Visibility {
		case "", "workspace_member", "private":
		default:
			writeErr(w, nethttp.StatusBadRequest, "kb.invalid_visibility", "visibility must be workspace_member or private")
			return
		}
		// integration_mode validation
		switch req.IntegrationMode {
		case "", "standalone":
		case "exam":
			if !h.examIntegrationEnabled {
				writeErr(w, nethttp.StatusBadRequest, "kb.integration_disabled",
					"本部署未启用 exam 集成，请联系管理员或选择独立模式")
				return
			}
			if req.ExamCourseID == nil || strings.TrimSpace(*req.ExamCourseID) == "" {
				writeErr(w, nethttp.StatusBadRequest, "kb.missing_exam_course",
					"选择绑定 exam 课程模式时必须指定 exam_course_id")
				return
			}
		default:
			writeErr(w, nethttp.StatusBadRequest, "kb.invalid_integration_mode",
				"integration_mode must be standalone or exam")
			return
		}
		if req.WorkspaceRef == "" {
			ws, err := h.ensureDefaultWorkspace(r.Context(), a)
			if err != nil {
				writeErr(w, nethttp.StatusInternalServerError, "internal.workspace_failed", "failed to resolve default workspace")
				return
			}
			req.WorkspaceRef = ws
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, req.WorkspaceRef); err != nil {
			writeErr(w, nethttp.StatusForbidden, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		kb, err := h.kbStore.Create(r.Context(), data.KBCreate{
			AccountID:       a.AccountID,
			WorkspaceRef:    req.WorkspaceRef,
			Name:            req.Name,
			Description:     req.Description,
			Visibility:      req.Visibility,
			IntegrationMode: req.IntegrationMode,
			ExamCourseID:    req.ExamCourseID,
			CreatedBy:       &a.UserID,
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
			var err error
			ws, err = h.ensureDefaultWorkspace(r.Context(), a)
			if err != nil {
				writeErr(w, nethttp.StatusInternalServerError, "internal.workspace_failed", "failed to resolve default workspace")
				return
			}
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
			kb := &kbs[i]
			if kb.Visibility == "private" && (kb.CreatedBy == nil || *kb.CreatedBy != a.UserID) {
				continue
			}
			items = append(items, kbResponse(kb))
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"items": items})
	}
}

func handleDefaultWorkspace(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		workspaceRef, err := h.ensureDefaultWorkspace(r.Context(), a)
		if err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.workspace_failed", "failed to resolve default workspace")
			return
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"workspace_ref": workspaceRef})
	}
}

func (h *handlerCtx) ensureDefaultWorkspace(ctx context.Context, a actor) (string, error) {
	if h == nil || h.profileRepo == nil || h.workspaceRepo == nil {
		return "", errors.New("workspace repos not configured")
	}
	profileRef := sharedenvironmentref.BuildProfileRef(a.AccountID, &a.UserID)
	if err := h.profileRepo.Ensure(ctx, profileRef, a.AccountID, a.UserID); err != nil {
		return "", err
	}
	profile, err := h.profileRepo.Get(ctx, profileRef)
	if err != nil {
		return "", err
	}
	if profile != nil && profile.DefaultWorkspaceRef != nil && strings.TrimSpace(*profile.DefaultWorkspaceRef) != "" {
		workspaceRef := strings.TrimSpace(*profile.DefaultWorkspaceRef)
		if err := h.workspaceRepo.Ensure(ctx, workspaceRef, a.AccountID, a.UserID, nil); err != nil {
			return "", err
		}
		return workspaceRef, nil
	}
	workspaceRef := "wsref_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := h.workspaceRepo.Ensure(ctx, workspaceRef, a.AccountID, a.UserID, nil); err != nil {
		return "", err
	}
	if err := h.profileRepo.SetDefaultWorkspaceRef(ctx, profileRef, workspaceRef); err != nil {
		return "", err
	}
	return workspaceRef, nil
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
	if !ensureKBVisible(w, kb, a) {
		return nil, false
	}
	return kb, true
}

func kbResponse(kb *data.KnowledgeBase) map[string]any {
	resp := map[string]any{
		"id":               kb.ID,
		"name":             kb.Name,
		"workspace_ref":    kb.WorkspaceRef,
		"description":      kb.Description,
		"visibility":       kb.Visibility,
		"integration_mode": kb.IntegrationMode,
		"document_count":   kb.DocumentCount,
		"created_at":       kb.CreatedAt,
		"updated_at":       kb.UpdatedAt,
	}
	if kb.ExamCourseID != nil {
		resp["exam_course_id"] = *kb.ExamCourseID
	}
	return resp
}

func (h *handlerCtx) withActor(next nethttp.HandlerFunc) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		a, ok := httpkit.ResolveActor(w, r, traceID, h.authService, h.accountMembershipRepo, h.apiKeysRepo, h.auditWriter)
		if !ok {
			return
		}
		next(w, injectActor(r, a.AccountID, a.UserID))
	}
}
