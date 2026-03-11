package orgapi

import (
	httpkit "arkloop/services/api/internal/http/httpkit"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	nethttp "net/http"
	"path"
	"sort"
	"strings"
	"time"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/shared/environmentref"
	"arkloop/services/shared/objectstore"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errProjectWorkspaceNotFound = errors.New("project workspace not found")

type projectWorkspaceStatus string

const (
	projectWorkspaceStatusActive      projectWorkspaceStatus = "active"
	projectWorkspaceStatusIdle        projectWorkspaceStatus = "idle"
	projectWorkspaceStatusUnavailable projectWorkspaceStatus = "unavailable"
)

type projectWorkspaceResponse struct {
	ProjectID     string                                 `json:"project_id"`
	WorkspaceRef  string                                 `json:"workspace_ref"`
	OwnerUserID   string                                 `json:"owner_user_id"`
	Status        projectWorkspaceStatus                 `json:"status"`
	LastUsedAt    string                                 `json:"last_used_at"`
	ActiveSession *projectWorkspaceActiveSessionResponse `json:"active_session,omitempty"`
}

type projectWorkspaceActiveSessionResponse struct {
	SessionRef  string `json:"session_ref"`
	SessionType string `json:"session_type"`
	State       string `json:"state"`
	LastUsedAt  string `json:"last_used_at"`
}

type projectWorkspaceFilesResponse struct {
	WorkspaceRef string                         `json:"workspace_ref"`
	Path         string                         `json:"path"`
	Items        []projectWorkspaceFileListItem `json:"items"`
}

type projectWorkspaceFileListItem struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	Type        string  `json:"type"`
	Size        *int64  `json:"size,omitempty"`
	MtimeUnixMs *int64  `json:"mtime_unix_ms,omitempty"`
	MimeType    *string `json:"mime_type,omitempty"`
	HasChildren bool    `json:"has_children,omitempty"`
}

type resolvedProjectWorkspace struct {
	Project      data.Project
	ProfileRef   string
	WorkspaceRef string
	Registry     *data.WorkspaceRegistry
	Session      *data.ShellSession
	Status       projectWorkspaceStatus
}

func handleProjectWorkspaceRoute(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	subpath string,
	projectID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	projectRepo *data.ProjectRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	pool *pgxpool.Pool,
	store environmentStore,
) {
	parts := strings.Split(strings.Trim(subpath, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		httpkit.WriteNotFound(w, r)
		return
	}
	if parts[0] != "workspace" {
		httpkit.WriteNotFound(w, r)
		return
	}

	switch {
	case len(parts) == 1 && r.Method == nethttp.MethodGet:
		getProjectWorkspace(w, r, traceID, projectID, authService, membershipRepo, projectRepo, apiKeysRepo, auditWriter, pool)
	case len(parts) == 2 && parts[1] == "files" && r.Method == nethttp.MethodGet:
		listProjectWorkspaceFiles(w, r, traceID, projectID, authService, membershipRepo, projectRepo, apiKeysRepo, auditWriter, pool, store)
	case len(parts) == 2 && parts[1] == "file" && r.Method == nethttp.MethodGet:
		getProjectWorkspaceFile(w, r, traceID, projectID, authService, membershipRepo, projectRepo, apiKeysRepo, auditWriter, pool, store)
	default:
		httpkit.WriteMethodNotAllowed(w, r)
	}
}

func getProjectWorkspace(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	projectID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	projectRepo *data.ProjectRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	pool *pgxpool.Pool,
) {
	actor, ok := resolveProjectWorkspaceActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, auditWriter, true)
	if !ok {
		return
	}
	resolved, ok := resolveProjectWorkspaceForActor(w, r, traceID, pool, projectRepo, projectID, actor)
	if !ok {
		return
	}
	if resolved.Registry == nil || resolved.Registry.OwnerUserID == nil {
		httpkit.WriteError(w, nethttp.StatusNotFound, "workspaces.not_found", "workspace not found", traceID, nil)
		return
	}

	resp := projectWorkspaceResponse{
		ProjectID:    resolved.Project.ID.String(),
		WorkspaceRef: resolved.WorkspaceRef,
		OwnerUserID:  resolved.Registry.OwnerUserID.String(),
		Status:       resolved.Status,
		LastUsedAt:   resolved.Registry.LastUsedAt.UTC().Format(time.RFC3339Nano),
	}
	if resolved.Session != nil && resolved.Status == projectWorkspaceStatusActive {
		resp.ActiveSession = &projectWorkspaceActiveSessionResponse{
			SessionRef:  resolved.Session.SessionRef,
			SessionType: resolved.Session.SessionType,
			State:       resolved.Session.State,
			LastUsedAt:  resolved.Session.LastUsedAt.UTC().Format(time.RFC3339Nano),
		}
	}
	httpkit.WriteJSON(w, traceID, nethttp.StatusOK, resp)
}

func listProjectWorkspaceFiles(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	projectID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	projectRepo *data.ProjectRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	pool *pgxpool.Pool,
	store environmentStore,
) {
	if store == nil {
		httpkit.WriteError(w, nethttp.StatusServiceUnavailable, "workspace_files.not_configured", "workspace file storage not configured", traceID, nil)
		return
	}
	actor, ok := resolveProjectWorkspaceActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, auditWriter, false)
	if !ok {
		return
	}
	resolved, ok := resolveProjectWorkspaceForActor(w, r, traceID, pool, projectRepo, projectID, actor)
	if !ok {
		return
	}
	relativePath, ok := normalizeWorkspaceDirectoryPath(w, traceID, r.URL.Query().Get("path"))
	if !ok {
		return
	}

	items, err := listWorkspaceManifestEntries(r.Context(), pool, store, resolved.WorkspaceRef, relativePath)
	if err != nil {
		if errors.Is(err, errWorkspaceFileNotFound) {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, projectWorkspaceFilesResponse{
				WorkspaceRef: resolved.WorkspaceRef,
				Path:         displayWorkspacePath(relativePath),
				Items:        []projectWorkspaceFileListItem{},
			})
			return
		}
		httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	httpkit.WriteJSON(w, traceID, nethttp.StatusOK, projectWorkspaceFilesResponse{
		WorkspaceRef: resolved.WorkspaceRef,
		Path:         displayWorkspacePath(relativePath),
		Items:        items,
	})
}

func getProjectWorkspaceFile(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	projectID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	projectRepo *data.ProjectRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	pool *pgxpool.Pool,
	store environmentStore,
) {
	if store == nil {
		httpkit.WriteError(w, nethttp.StatusServiceUnavailable, "workspace_files.not_configured", "workspace file storage not configured", traceID, nil)
		return
	}
	actor, ok := resolveProjectWorkspaceActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, auditWriter, false)
	if !ok {
		return
	}
	resolved, ok := resolveProjectWorkspaceForActor(w, r, traceID, pool, projectRepo, projectID, actor)
	if !ok {
		return
	}
	relativePath, ok := normalizeWorkspaceRelativePath(w, traceID, r.URL.Query().Get("path"))
	if !ok {
		return
	}

	content, contentType, err := readWorkspaceFile(r.Context(), pool, store, resolved.WorkspaceRef, relativePath)
	if err != nil {
		if errors.Is(err, errWorkspaceFileNotFound) {
			httpkit.WriteError(w, nethttp.StatusNotFound, "workspace_files.not_found", "workspace file not found", traceID, nil)
			return
		}
		httpkit.WriteError(w, nethttp.StatusBadRequest, "workspace_files.invalid_path", "invalid workspace path", traceID, nil)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write(content)
}

func resolveProjectWorkspaceActor(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	apiKeysRepo *data.APIKeysRepository,
	auditWriter *audit.Writer,
	workspaceOnly bool,
) (*httpkit.Actor, bool) {
	if authService == nil {
		httpkit.WriteAuthNotConfigured(w, traceID)
		return nil, false
	}
	actor, ok := httpkit.ResolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, auditWriter)
	if !ok {
		return nil, false
	}
	if !httpkit.RequirePerm(actor, auth.PermDataProjectsRead, w, traceID) {
		return nil, false
	}
	if !workspaceOnly && !httpkit.RequirePerm(actor, auth.PermDataRunsRead, w, traceID) {
		return nil, false
	}
	return actor, true
}

func resolveProjectWorkspaceForActor(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	pool *pgxpool.Pool,
	projectRepo *data.ProjectRepository,
	projectID uuid.UUID,
	actor *httpkit.Actor,
) (*resolvedProjectWorkspace, bool) {
	if pool == nil || projectRepo == nil {
		httpkit.WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return nil, false
	}
	resolved, err := resolveProjectWorkspace(r.Context(), pool, projectRepo, projectID, actor)
	if err != nil {
		if errors.Is(err, errProjectWorkspaceNotFound) {
			httpkit.WriteError(w, nethttp.StatusNotFound, "projects.not_found", "project not found", traceID, nil)
			return nil, false
		}
		httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return nil, false
	}
	return resolved, true
}

func resolveProjectWorkspace(
	ctx context.Context,
	pool *pgxpool.Pool,
	projectRepo *data.ProjectRepository,
	projectID uuid.UUID,
	actor *httpkit.Actor,
) (*resolvedProjectWorkspace, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil || projectRepo == nil || actor == nil {
		return nil, fmt.Errorf("project workspace dependencies must not be nil")
	}
	project, err := projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil || project.OrgID != actor.OrgID {
		return nil, errProjectWorkspaceNotFound
	}

	profileRef := environmentref.BuildProfileRef(actor.OrgID, &actor.UserID)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	bindingsRepo, err := data.NewDefaultWorkspaceBindingsRepository(tx)
	if err != nil {
		return nil, err
	}
	profileRepo, err := data.NewProfileRegistriesRepository(tx)
	if err != nil {
		return nil, err
	}
	workspaceRepo, err := data.NewWorkspaceRegistriesRepository(tx)
	if err != nil {
		return nil, err
	}

	candidate := environmentref.BuildWorkspaceRef(actor.OrgID, profileRef, data.DefaultWorkspaceBindingScopeProject, projectID)
	workspaceRef, _, err := bindingsRepo.GetOrCreate(ctx, tx, actor.OrgID, &actor.UserID, profileRef, data.DefaultWorkspaceBindingScopeProject, projectID, candidate)
	if err != nil {
		return nil, err
	}
	if err := profileRepo.Ensure(ctx, profileRef, actor.OrgID, actor.UserID); err != nil {
		return nil, err
	}
	if err := workspaceRepo.Ensure(ctx, workspaceRef, actor.OrgID, actor.UserID, &projectID); err != nil {
		return nil, err
	}
	if err := profileRepo.SetDefaultWorkspaceRef(ctx, profileRef, workspaceRef); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	workspaceRepo, err = data.NewWorkspaceRegistriesRepository(pool)
	if err != nil {
		return nil, err
	}
	sessionRepo, err := data.NewShellSessionRepository(pool)
	if err != nil {
		return nil, err
	}
	registry, err := workspaceRepo.Get(ctx, workspaceRef)
	if err != nil {
		return nil, err
	}
	if registry == nil {
		return &resolvedProjectWorkspace{
			Project:      *project,
			ProfileRef:   profileRef,
			WorkspaceRef: workspaceRef,
			Status:       projectWorkspaceStatusUnavailable,
		}, nil
	}

	resolved := &resolvedProjectWorkspace{
		Project:      *project,
		ProfileRef:   profileRef,
		WorkspaceRef: workspaceRef,
		Registry:     registry,
		Status:       projectWorkspaceStatusIdle,
	}
	if registry.DefaultShellSessionRef == nil || strings.TrimSpace(*registry.DefaultShellSessionRef) == "" {
		return resolved, nil
	}
	resolvedSession, err := sessionRepo.GetBySessionRef(ctx, actor.OrgID, strings.TrimSpace(*registry.DefaultShellSessionRef))
	if err != nil {
		return nil, err
	}
	if resolvedSession == nil || strings.TrimSpace(resolvedSession.WorkspaceRef) != workspaceRef {
		resolved.Status = projectWorkspaceStatusUnavailable
		return resolved, nil
	}
	if strings.TrimSpace(resolvedSession.State) == "closed" {
		return resolved, nil
	}
	resolved.Session = resolvedSession
	if resolvedSession.LiveSessionID != nil && strings.TrimSpace(*resolvedSession.LiveSessionID) != "" {
		resolved.Status = projectWorkspaceStatusActive
	}
	return resolved, nil
}

func normalizeWorkspaceDirectoryPath(w nethttp.ResponseWriter, traceID string, raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "/" {
		return "", true
	}
	cleaned := path.Clean(path.Join(workspaceRootPath, strings.TrimPrefix(trimmed, "/")))
	if cleaned == workspaceRootPath {
		return "", true
	}
	if !strings.HasPrefix(cleaned, workspaceRootPath+"/") {
		httpkit.WriteError(w, nethttp.StatusBadRequest, "workspace_files.invalid_path", "invalid workspace path", traceID, nil)
		return "", false
	}
	return strings.TrimPrefix(strings.TrimPrefix(cleaned, workspaceRootPath), "/"), true
}

func displayWorkspacePath(relativePath string) string {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return "/"
	}
	return "/" + relativePath
}

func loadWorkspaceManifest(ctx context.Context, pool *pgxpool.Pool, store environmentStore, workspaceRef string) (workspaceManifest, error) {
	revision, err := loadWorkspaceManifestRevision(ctx, pool, workspaceRef)
	if err != nil {
		return workspaceManifest{}, err
	}
	if revision == "" {
		return workspaceManifest{}, nil
	}
	manifestBytes, err := store.Get(ctx, workspaceManifestKey(workspaceRef, revision))
	if err != nil {
		if objectstore.IsNotFound(err) {
			return workspaceManifest{}, errWorkspaceFileNotFound
		}
		return workspaceManifest{}, err
	}
	var manifest workspaceManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return workspaceManifest{}, err
	}
	return manifest, nil
}

func listWorkspaceManifestEntries(
	ctx context.Context,
	pool *pgxpool.Pool,
	store environmentStore,
	workspaceRef string,
	relativePath string,
) ([]projectWorkspaceFileListItem, error) {
	manifest, err := loadWorkspaceManifest(ctx, pool, store, workspaceRef)
	if err != nil {
		return nil, err
	}
	if len(manifest.Entries) == 0 {
		return []projectWorkspaceFileListItem{}, nil
	}

	itemsByPath := make(map[string]*projectWorkspaceFileListItem)
	prefix := ""
	if relativePath != "" {
		prefix = relativePath + "/"
	}
	for _, entry := range manifest.Entries {
		entryPath := strings.Trim(strings.TrimSpace(entry.Path), "/")
		if entryPath == "" || entry.Deleted {
			continue
		}
		if relativePath != "" {
			if entryPath == relativePath {
				continue
			}
			if !strings.HasPrefix(entryPath, prefix) {
				continue
			}
		}

		remainder := entryPath
		if prefix != "" {
			remainder = strings.TrimPrefix(entryPath, prefix)
		}
		if remainder == "" {
			continue
		}
		childName, childTail, hasMore := strings.Cut(remainder, "/")
		childPath := childName
		if relativePath != "" {
			childPath = relativePath + "/" + childName
		}
		item, ok := itemsByPath[childPath]
		if !ok {
			item = &projectWorkspaceFileListItem{
				Name: childName,
				Path: displayWorkspacePath(childPath),
			}
			itemsByPath[childPath] = item
		}
		if hasMore || strings.TrimSpace(childTail) != "" {
			item.Type = workspaceEntryTypeDir
			item.HasChildren = true
			continue
		}
		switch entry.Type {
		case workspaceEntryTypeDir:
			item.Type = workspaceEntryTypeDir
		case workspaceEntryTypeFile, workspaceEntryTypeSymlink:
			item.Type = entry.Type
			item.Size = int64Ptr(entry.Size)
			item.MtimeUnixMs = int64Ptr(entry.MtimeUnixMs)
			mimeType := guessWorkspaceListMimeType(childName)
			item.MimeType = &mimeType
		}
	}

	items := make([]projectWorkspaceFileListItem, 0, len(itemsByPath))
	for _, item := range itemsByPath {
		if strings.TrimSpace(item.Type) == "" {
			item.Type = workspaceEntryTypeDir
		}
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			if items[i].Type == workspaceEntryTypeDir {
				return true
			}
			if items[j].Type == workspaceEntryTypeDir {
				return false
			}
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func guessWorkspaceListMimeType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ext != "" {
		if guessed := mime.TypeByExtension(ext); strings.TrimSpace(guessed) != "" {
			return guessed
		}
	}
	return "application/octet-stream"
}

func int64Ptr(value int64) *int64 {
	copied := value
	return &copied
}
