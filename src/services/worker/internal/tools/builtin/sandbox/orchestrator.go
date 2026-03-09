package sandbox

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/tools"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sessionModeAuto   = "auto"
	sessionModeNew    = "new"
	sessionModeResume = "resume"
	sessionModeFork   = "fork"

	minimumWriterLeaseTTL = 2 * time.Minute
	execWriterLeasePad    = 5 * time.Second
)

type resolvedSession struct {
	SessionRef               string
	ResolvedVia              string
	Reused                   bool
	RestoredFromRestoreState bool
	FromSessionRef           string
	ShareScope               string
	OpenMode                 string
	AllowUnavailableFallback bool
	DefaultBindingKey        *string
	Record                   *data.ShellSessionRecord
}

type sessionOrchestrator struct {
	pool            *pgxpool.Pool
	sessionsRepo    data.ShellSessionsRepository
	registryService *registryService
	acl             *sessionACLEvaluator

	mu             sync.Mutex
	memorySessions map[string]data.ShellSessionRecord
}

func newSessionOrchestrator(pool *pgxpool.Pool) *sessionOrchestrator {
	return &sessionOrchestrator{
		pool:            pool,
		registryService: newRegistryService(pool),
		acl:             newSessionACLEvaluator(pool),
		memorySessions:  map[string]data.ShellSessionRecord{},
	}
}

func (o *sessionOrchestrator) resolveExecSession(
	ctx context.Context,
	req execCommandArgs,
	execCtx tools.ExecutionContext,
) (*resolvedSession, *tools.ExecutionError) {
	mode := normalizeSessionMode(req.SessionMode)
	shareScope, err := normalizeRequestedShareScope(req.ShareScope)
	if err != nil {
		return nil, sandboxArgsError(err.Error())
	}
	if mode == sessionModeResume && strings.TrimSpace(req.SessionRef) == "" {
		return nil, sandboxArgsError("parameter session_ref is required when session_mode=resume")
	}
	if mode == sessionModeFork && strings.TrimSpace(req.FromSessionRef) == "" {
		return nil, sandboxArgsError("parameter from_session_ref is required when session_mode=fork")
	}
	if shareScope != "" && mode == sessionModeAuto && strings.TrimSpace(req.SessionRef) != "" {
		return nil, sandboxArgsError("parameter share_scope is not supported when session_ref is provided")
	}
	if shareScope != "" && mode == sessionModeResume {
		return nil, sandboxArgsError("parameter share_scope is not supported when session_mode=resume")
	}
	if shareScope != "" && mode == sessionModeFork {
		return nil, sandboxArgsError("parameter share_scope is not supported when session_mode=fork")
	}

	if strings.TrimSpace(req.SessionRef) != "" && mode == sessionModeAuto {
		return o.lookupExplicit(ctx, execCtx, strings.TrimSpace(req.SessionRef), "explicit_resume")
	}

	switch mode {
	case sessionModeResume:
		return o.lookupExplicit(ctx, execCtx, strings.TrimSpace(req.SessionRef), "explicit_resume")
	case sessionModeFork:
		base, err := o.lookupExplicit(ctx, execCtx, strings.TrimSpace(req.FromSessionRef), "fork_from_restore_state")
		if err != nil {
			return nil, err
		}
		created, createErr := o.createSession(ctx, execCtx, base.ShareScope, nil)
		if createErr != nil {
			return nil, createErr
		}
		created.ResolvedVia = "fork_from_restore_state"
		created.FromSessionRef = base.SessionRef
		created.RestoredFromRestoreState = true
		return created, nil
	case sessionModeNew:
		resolvedShareScope := requestedShareScopeOrDefault(execCtx, shareScope)
		created, err := o.createSession(ctx, execCtx, resolvedShareScope, defaultBindingKeyForShareScope(execCtx, resolvedShareScope))
		if err != nil {
			return nil, err
		}
		created.ResolvedVia = "new_session"
		return created, nil
	default:
		if resolved := o.lookupRunDefault(ctx, execCtx, ""); resolved != nil {
			resolved.Reused = true
			resolved.ResolvedVia = "run_default"
			resolved.AllowUnavailableFallback = true
			return resolved, nil
		}
		if resolved := o.lookupDefaultBinding(ctx, execCtx, data.ShellDefaultBindingKeyForThread(execCtx.ThreadID), "", "thread_default"); resolved != nil {
			resolved.Reused = true
			resolved.AllowUnavailableFallback = true
			return resolved, nil
		}
		if resolved := o.lookupDefaultBinding(ctx, execCtx, data.ShellDefaultBindingKeyForWorkspace(execCtx.WorkspaceRef), "", "workspace_default"); resolved != nil {
			resolved.Reused = true
			resolved.AllowUnavailableFallback = true
			return resolved, nil
		}

		resolvedShareScope := requestedShareScopeOrDefault(execCtx, shareScope)
		defaultBindingKey := defaultBindingKeyForShareScope(execCtx, resolvedShareScope)
		created, err := o.createSession(ctx, execCtx, resolvedShareScope, defaultBindingKey)
		if err != nil {
			return nil, err
		}
		created.ResolvedVia = "new_session"
		return created, nil
	}
}

func (o *sessionOrchestrator) resolveFallbackSession(
	ctx context.Context,
	req execCommandArgs,
	execCtx tools.ExecutionContext,
	failed *resolvedSession,
) (*resolvedSession, *tools.ExecutionError) {
	if failed == nil || !failed.AllowUnavailableFallback {
		return nil, nil
	}
	if err := o.clearLiveSession(ctx, execCtx, failed.SessionRef); err != nil && !data.IsShellSessionNotFound(err) {
		return nil, sandboxArgsError(err.Error())
	}
	skip := failed.SessionRef
	switch failed.ResolvedVia {
	case "run_default":
		if resolved := o.lookupDefaultBinding(ctx, execCtx, data.ShellDefaultBindingKeyForThread(execCtx.ThreadID), skip, "thread_default"); resolved != nil {
			resolved.Reused = true
			resolved.AllowUnavailableFallback = true
			return resolved, nil
		}
		if resolved := o.lookupDefaultBinding(ctx, execCtx, data.ShellDefaultBindingKeyForWorkspace(execCtx.WorkspaceRef), skip, "workspace_default"); resolved != nil {
			resolved.Reused = true
			resolved.AllowUnavailableFallback = true
			return resolved, nil
		}
	case "thread_default":
		if resolved := o.lookupDefaultBinding(ctx, execCtx, data.ShellDefaultBindingKeyForWorkspace(execCtx.WorkspaceRef), skip, "workspace_default"); resolved != nil {
			resolved.Reused = true
			resolved.AllowUnavailableFallback = true
			return resolved, nil
		}
	case "workspace_default":
	}
	shareScope := requestedShareScopeOrDefault(execCtx, strings.TrimSpace(req.ShareScope))
	defaultBindingKey := defaultBindingKeyForShareScope(execCtx, shareScope)
	created, err := o.createSession(ctx, execCtx, shareScope, defaultBindingKey)
	if err != nil {
		return nil, err
	}
	created.ResolvedVia = "new_session"
	return created, nil
}

func (o *sessionOrchestrator) resolveWriteSession(
	ctx context.Context,
	req writeStdinArgs,
	execCtx tools.ExecutionContext,
) (*resolvedSession, *tools.ExecutionError) {
	if strings.TrimSpace(req.SessionRef) == "" {
		return nil, sandboxArgsError("parameter session_ref is required")
	}
	return o.lookupExplicit(ctx, execCtx, strings.TrimSpace(req.SessionRef), "explicit_resume")
}

func (o *sessionOrchestrator) lookupExplicit(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	sessionRef string,
	resolvedVia string,
) (*resolvedSession, *tools.ExecutionError) {
	record, found, err := o.lookupSession(ctx, execCtx, sessionRef)
	if err != nil {
		return nil, sandboxArgsError(err.Error())
	}
	if !found {
		return nil, &tools.ExecutionError{ErrorClass: errorSandboxError, Message: "shell session not found", Details: map[string]any{"session_ref": sessionRef}}
	}
	if aclErr := o.acl.AuthorizeSession(ctx, execCtx, record); aclErr != nil {
		return nil, aclErr
	}
	return &resolvedSession{
		SessionRef:               sessionRef,
		ResolvedVia:              resolvedVia,
		Reused:                   true,
		ShareScope:               record.ShareScope,
		OpenMode:                 openModeAttachOrRestore,
		DefaultBindingKey:        record.DefaultBindingKey,
		Record:                   &record,
		RestoredFromRestoreState: record.LiveSessionID == nil && strings.TrimSpace(stringPtrValue(record.LatestRestoreRev)) != "",
	}, nil
}

func (o *sessionOrchestrator) lookupRunDefault(ctx context.Context, execCtx tools.ExecutionContext, skipSessionRef string) *resolvedSession {
	if execCtx.RunID == uuid.Nil {
		return nil
	}
	record, found, err := o.lookupLatestByRun(ctx, derefUUID(execCtx.OrgID), execCtx.RunID)
	if err != nil || !found || record.SessionRef == strings.TrimSpace(skipSessionRef) {
		return nil
	}
	if o.acl.AuthorizeSession(ctx, execCtx, record) != nil {
		return nil
	}
	return &resolvedSession{
		SessionRef:        record.SessionRef,
		ShareScope:        record.ShareScope,
		OpenMode:          openModeAttachOrRestore,
		DefaultBindingKey: record.DefaultBindingKey,
		Record:            &record,
	}
}

func (o *sessionOrchestrator) lookupDefaultBinding(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	defaultBindingKey string,
	skipSessionRef string,
	resolvedVia string,
) *resolvedSession {
	orgID := derefUUID(execCtx.OrgID)
	profileRef := strings.TrimSpace(execCtx.ProfileRef)
	defaultBindingKey = strings.TrimSpace(defaultBindingKey)
	if orgID == uuid.Nil || profileRef == "" || defaultBindingKey == "" {
		return nil
	}
	record, found, err := o.lookupByDefaultBindingKey(ctx, orgID, profileRef, defaultBindingKey)
	if err != nil || !found || record.SessionRef == strings.TrimSpace(skipSessionRef) {
		return nil
	}
	if o.acl.AuthorizeSession(ctx, execCtx, record) != nil {
		return nil
	}
	return &resolvedSession{
		SessionRef:        record.SessionRef,
		ResolvedVia:       resolvedVia,
		ShareScope:        record.ShareScope,
		OpenMode:          openModeAttachOrRestore,
		DefaultBindingKey: record.DefaultBindingKey,
		Record:            &record,
	}
}

func (o *sessionOrchestrator) createSession(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	shareScope string,
	defaultBindingKey *string,
) (*resolvedSession, *tools.ExecutionError) {
	if aclErr := o.acl.AuthorizeShareScopeCreation(ctx, execCtx, shareScope); aclErr != nil {
		return nil, aclErr
	}
	sessionRef := newSessionRef()
	record := data.ShellSessionRecord{
		SessionRef:        sessionRef,
		OrgID:             derefUUID(execCtx.OrgID),
		ProfileRef:        strings.TrimSpace(execCtx.ProfileRef),
		WorkspaceRef:      strings.TrimSpace(execCtx.WorkspaceRef),
		ProjectID:         uuidPtr(execCtx.ProjectID),
		ThreadID:          execCtx.ThreadID,
		RunID:             uuidPtr(execCtx.RunID),
		ShareScope:        shareScope,
		State:             data.ShellSessionStateReady,
		DefaultBindingKey: defaultBindingKey,
		MetadataJSON:      map[string]any{},
	}
	if o.pool != nil {
		if record.OrgID == uuid.Nil {
			return nil, sandboxArgsError("org_id is required for shell sessions")
		}
		if record.ProfileRef == "" || record.WorkspaceRef == "" {
			return nil, sandboxArgsError("profile_ref and workspace_ref are required for shell sessions")
		}
	}
	if err := o.saveSession(ctx, execCtx, record); err != nil {
		return nil, sandboxArgsError(err.Error())
	}
	return &resolvedSession{
		SessionRef:        sessionRef,
		ResolvedVia:       "new_session",
		Reused:            false,
		ShareScope:        shareScope,
		OpenMode:          openModeCreate,
		DefaultBindingKey: defaultBindingKey,
		Record:            &record,
	}, nil
}

func (o *sessionOrchestrator) lookupLatestByRun(
	ctx context.Context,
	orgID uuid.UUID,
	runID uuid.UUID,
) (data.ShellSessionRecord, bool, error) {
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		var selected data.ShellSessionRecord
		found := false
		for _, record := range o.memorySessions {
			if record.OrgID != orgID || record.RunID == nil || *record.RunID != runID || record.State == data.ShellSessionStateClosed {
				continue
			}
			if !found || record.LastUsedAt.After(selected.LastUsedAt) || (record.LastUsedAt.Equal(selected.LastUsedAt) && record.UpdatedAt.After(selected.UpdatedAt)) {
				selected = record
				found = true
			}
		}
		return selected, found, nil
	}
	record, err := o.sessionsRepo.GetLatestByRun(ctx, o.pool, orgID, runID)
	if err != nil {
		if data.IsShellSessionNotFound(err) {
			return data.ShellSessionRecord{}, false, nil
		}
		return data.ShellSessionRecord{}, false, err
	}
	return record, true, nil
}

func (o *sessionOrchestrator) lookupByDefaultBindingKey(
	ctx context.Context,
	orgID uuid.UUID,
	profileRef string,
	defaultBindingKey string,
) (data.ShellSessionRecord, bool, error) {
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		var selected data.ShellSessionRecord
		found := false
		for _, record := range o.memorySessions {
			if record.OrgID != orgID || strings.TrimSpace(record.ProfileRef) != strings.TrimSpace(profileRef) {
				continue
			}
			if strings.TrimSpace(stringPtrValue(record.DefaultBindingKey)) != strings.TrimSpace(defaultBindingKey) || record.State == data.ShellSessionStateClosed {
				continue
			}
			if !found || record.LastUsedAt.After(selected.LastUsedAt) || (record.LastUsedAt.Equal(selected.LastUsedAt) && record.UpdatedAt.After(selected.UpdatedAt)) {
				selected = record
				found = true
			}
		}
		return selected, found, nil
	}
	record, err := o.sessionsRepo.GetByDefaultBindingKey(ctx, o.pool, orgID, profileRef, defaultBindingKey)
	if err != nil {
		if data.IsShellSessionNotFound(err) {
			return data.ShellSessionRecord{}, false, nil
		}
		return data.ShellSessionRecord{}, false, err
	}
	return record, true, nil
}

func (o *sessionOrchestrator) lookupSession(ctx context.Context, execCtx tools.ExecutionContext, sessionRef string) (data.ShellSessionRecord, bool, error) {
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		record, ok := o.memorySessions[sessionRef]
		if ok {
			return record, true, nil
		}
		return data.ShellSessionRecord{}, false, nil
	}
	record, err := o.sessionsRepo.GetBySessionRef(ctx, o.pool, derefUUID(execCtx.OrgID), sessionRef)
	if err != nil {
		if data.IsShellSessionNotFound(err) {
			return data.ShellSessionRecord{}, false, nil
		}
		return data.ShellSessionRecord{}, false, err
	}
	return record, true, nil
}

func (o *sessionOrchestrator) saveSession(ctx context.Context, execCtx tools.ExecutionContext, record data.ShellSessionRecord) error {
	now := time.Now().UTC()
	record.LastUsedAt = now
	record.UpdatedAt = now
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.memorySessions[record.SessionRef] = record
		return nil
	}
	if err := o.registryService.UpsertProfileRegistry(ctx, record.OrgID, execCtx.UserID, record.ProfileRef, stringPtr(record.WorkspaceRef)); err != nil {
		return err
	}
	if err := o.registryService.UpsertWorkspaceRegistry(ctx, record.OrgID, execCtx.UserID, execCtx.ProjectID, record.WorkspaceRef, nil); err != nil {
		return err
	}
	return o.sessionsRepo.Upsert(ctx, o.pool, record)
}

func (o *sessionOrchestrator) clearLiveSession(ctx context.Context, execCtx tools.ExecutionContext, sessionRef string) error {
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		record, ok := o.memorySessions[sessionRef]
		if !ok {
			return nil
		}
		record.LiveSessionID = nil
		record.LeaseOwnerID = nil
		record.LeaseUntil = nil
		record.State = data.ShellSessionStateReady
		record.LastUsedAt = time.Now().UTC()
		record.UpdatedAt = record.LastUsedAt
		o.memorySessions[sessionRef] = record
		return nil
	}
	return o.sessionsRepo.ClearLiveSession(ctx, o.pool, derefUUID(execCtx.OrgID), sessionRef)
}

func (o *sessionOrchestrator) markResult(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	resp execSessionResponse,
) {
	if resolution == nil {
		return
	}
	orgID := derefUUID(execCtx.OrgID)
	state := data.ShellSessionStateReady
	if resp.Running {
		state = data.ShellSessionStateBusy
	}
	record := data.ShellSessionRecord{
		SessionRef:        resolution.SessionRef,
		OrgID:             orgID,
		ProfileRef:        strings.TrimSpace(execCtx.ProfileRef),
		WorkspaceRef:      strings.TrimSpace(execCtx.WorkspaceRef),
		ProjectID:         uuidPtr(execCtx.ProjectID),
		ThreadID:          execCtx.ThreadID,
		RunID:             uuidPtr(execCtx.RunID),
		ShareScope:        resolution.ShareScope,
		State:             state,
		LiveSessionID:     stringPtr(resolution.SessionRef),
		DefaultBindingKey: resolution.DefaultBindingKey,
		MetadataJSON:      map[string]any{},
	}
	if resolution.Record != nil {
		record = *resolution.Record
		record.OrgID = orgID
		record.ProfileRef = strings.TrimSpace(execCtx.ProfileRef)
		record.WorkspaceRef = strings.TrimSpace(execCtx.WorkspaceRef)
		record.ProjectID = uuidPtr(execCtx.ProjectID)
		record.ThreadID = execCtx.ThreadID
		record.RunID = uuidPtr(execCtx.RunID)
		record.ShareScope = resolution.ShareScope
		record.State = state
		record.LiveSessionID = stringPtr(resolution.SessionRef)
		record.DefaultBindingKey = resolution.DefaultBindingKey
		if record.MetadataJSON == nil {
			record.MetadataJSON = map[string]any{}
		}
	}
	if !resp.Running {
		record.LeaseOwnerID = nil
		record.LeaseUntil = nil
	}
	if strings.TrimSpace(resp.RestoreRevision) != "" {
		record.LatestRestoreRev = stringPtr(strings.TrimSpace(resp.RestoreRevision))
	}
	record.LastUsedAt = time.Now().UTC()
	record.UpdatedAt = record.LastUsedAt
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.memorySessions[resolution.SessionRef] = record
		return
	}
	if err := o.sessionsRepo.Upsert(ctx, o.pool, record); err != nil {
		return
	}
	if err := o.registryService.UpsertProfileRegistry(ctx, orgID, execCtx.UserID, record.ProfileRef, stringPtr(record.WorkspaceRef)); err != nil {
		return
	}
	var defaultShellSessionRef *string
	if strings.HasPrefix(stringPtrValue(record.DefaultBindingKey), "workspace:") {
		defaultShellSessionRef = stringPtr(record.SessionRef)
	}
	_ = o.registryService.UpsertWorkspaceRegistry(ctx, orgID, execCtx.UserID, execCtx.ProjectID, record.WorkspaceRef, defaultShellSessionRef)
}

func normalizeSessionMode(value string) string {
	switch strings.TrimSpace(value) {
	case sessionModeNew, sessionModeResume, sessionModeFork:
		return strings.TrimSpace(value)
	default:
		return sessionModeAuto
	}
}

func defaultShareScope(execCtx tools.ExecutionContext) string {
	if execCtx.ThreadID != nil && *execCtx.ThreadID != uuid.Nil {
		return data.ShellShareScopeThread
	}
	return data.ShellShareScopeWorkspace
}

func requestedShareScopeOrDefault(execCtx tools.ExecutionContext, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested
	}
	return defaultShareScope(execCtx)
}

func normalizeRequestedShareScope(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "", nil
	case data.ShellShareScopeRun, data.ShellShareScopeThread, data.ShellShareScopeWorkspace, data.ShellShareScopeOrg:
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("parameter share_scope must be one of run, thread, workspace, org")
	}
}

func defaultBindingKeyForShareScope(execCtx tools.ExecutionContext, shareScope string) *string {
	var value string
	switch shareScope {
	case data.ShellShareScopeRun:
		return nil
	case data.ShellShareScopeThread:
		value = data.ShellDefaultBindingKeyForThread(execCtx.ThreadID)
	case data.ShellShareScopeWorkspace:
		value = data.ShellDefaultBindingKeyForWorkspace(execCtx.WorkspaceRef)
	}
	return stringPtr(value)
}

func newSessionRef() string {
	return "shref_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func derefUUID(value *uuid.UUID) uuid.UUID {
	if value == nil {
		return uuid.Nil
	}
	return *value
}

func uuidPtr(value any) *uuid.UUID {
	switch typed := value.(type) {
	case uuid.UUID:
		if typed == uuid.Nil {
			return nil
		}
		copied := typed
		return &copied
	case *uuid.UUID:
		if typed == nil || *typed == uuid.Nil {
			return nil
		}
		copied := *typed
		return &copied
	default:
		return nil
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	copied := value
	return &copied
}

func (o *sessionOrchestrator) prepareExecWriterLease(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	timeoutMs int,
) *tools.ExecutionError {
	if resolution == nil {
		return nil
	}
	record := o.sessionRecord(execCtx, resolution)
	if record.State == data.ShellSessionStateBusy && hasActiveWriterLease(record, time.Now().UTC()) {
		return shellBusyError(resolution.SessionRef)
	}
	updated, err := o.acquireWriterLease(ctx, execCtx, resolution, writerLeaseOwner(execCtx), execWriterLeaseUntil(timeoutMs))
	if err != nil {
		return err
	}
	resolution.Record = &updated
	return nil
}

func (o *sessionOrchestrator) prepareWriteWriterLease(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	hasInput bool,
) *tools.ExecutionError {
	if resolution == nil || !hasInput {
		return nil
	}
	ownerID := writerLeaseOwner(execCtx)
	updated, err := o.acquireWriterLease(ctx, execCtx, resolution, ownerID, writeWriterLeaseUntil())
	if err != nil {
		return err
	}
	resolution.Record = &updated
	return nil
}

func (o *sessionOrchestrator) reconcileWriteWriterLease(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	hasInput bool,
	resp execSessionResponse,
) {
	if resolution == nil {
		return
	}
	if !resp.Running {
		_ = o.clearFinishedWriterLease(ctx, execCtx, resolution)
		return
	}
	if hasInput {
		return
	}
	ownerID := writerLeaseOwner(execCtx)
	if strings.TrimSpace(stringPtrValue(resolution.RecordLeaseOwnerID())) != ownerID {
		return
	}
	updated, err := o.renewWriterLease(ctx, execCtx, resolution, ownerID, writeWriterLeaseUntil())
	if err != nil {
		return
	}
	resolution.Record = &updated
}

func (o *sessionOrchestrator) releaseWriterLease(ctx context.Context, execCtx tools.ExecutionContext, resolution *resolvedSession, ownerID string) {
	if resolution == nil || strings.TrimSpace(ownerID) == "" {
		return
	}
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		record, ok := o.memorySessions[resolution.SessionRef]
		if !ok || strings.TrimSpace(stringPtrValue(record.LeaseOwnerID)) != strings.TrimSpace(ownerID) {
			return
		}
		record.LeaseOwnerID = nil
		record.LeaseUntil = nil
		record.LastUsedAt = time.Now().UTC()
		record.UpdatedAt = record.LastUsedAt
		o.memorySessions[resolution.SessionRef] = record
		resolution.Record = &record
		return
	}
	if err := o.sessionsRepo.ReleaseWriterLease(ctx, o.pool, derefUUID(execCtx.OrgID), resolution.SessionRef, ownerID); err != nil {
		return
	}
	if resolution.Record != nil {
		resolution.Record.LeaseOwnerID = nil
		resolution.Record.LeaseUntil = nil
	}
}

func (o *sessionOrchestrator) clearFinishedWriterLease(ctx context.Context, execCtx tools.ExecutionContext, resolution *resolvedSession) error {
	if resolution == nil {
		return nil
	}
	if o.pool == nil {
		o.mu.Lock()
		defer o.mu.Unlock()
		record, ok := o.memorySessions[resolution.SessionRef]
		if !ok {
			return nil
		}
		record.LeaseOwnerID = nil
		record.LeaseUntil = nil
		record.State = data.ShellSessionStateReady
		record.LastUsedAt = time.Now().UTC()
		record.UpdatedAt = record.LastUsedAt
		o.memorySessions[resolution.SessionRef] = record
		resolution.Record = &record
		return nil
	}
	if err := o.sessionsRepo.ClearFinishedWriterLease(ctx, o.pool, derefUUID(execCtx.OrgID), resolution.SessionRef); err != nil && !data.IsShellSessionNotFound(err) {
		return err
	}
	if resolution.Record != nil {
		resolution.Record.LeaseOwnerID = nil
		resolution.Record.LeaseUntil = nil
		resolution.Record.State = data.ShellSessionStateReady
	}
	return nil
}

func (o *sessionOrchestrator) acquireWriterLease(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	ownerID string,
	leaseUntil time.Time,
) (data.ShellSessionRecord, *tools.ExecutionError) {
	return o.updateWriterLease(ctx, execCtx, resolution, ownerID, leaseUntil, false)
}

func (o *sessionOrchestrator) renewWriterLease(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	ownerID string,
	leaseUntil time.Time,
) (data.ShellSessionRecord, *tools.ExecutionError) {
	return o.updateWriterLease(ctx, execCtx, resolution, ownerID, leaseUntil, true)
}

func (o *sessionOrchestrator) updateWriterLease(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	ownerID string,
	leaseUntil time.Time,
	renewOnly bool,
) (data.ShellSessionRecord, *tools.ExecutionError) {
	ownerID = strings.TrimSpace(ownerID)
	if resolution == nil || ownerID == "" {
		return data.ShellSessionRecord{}, sandboxArgsError("run_id is required for shell writer lease")
	}
	if o.pool == nil {
		return o.updateMemoryWriterLease(execCtx, resolution, ownerID, leaseUntil, renewOnly)
	}
	var (
		record data.ShellSessionRecord
		err    error
	)
	if renewOnly {
		record, err = o.sessionsRepo.RenewWriterLease(ctx, o.pool, derefUUID(execCtx.OrgID), resolution.SessionRef, ownerID, leaseUntil)
	} else {
		record, err = o.sessionsRepo.AcquireWriterLease(ctx, o.pool, derefUUID(execCtx.OrgID), resolution.SessionRef, ownerID, leaseUntil)
	}
	if err == nil {
		return record, nil
	}
	if data.IsShellSessionLeaseConflict(err) {
		return data.ShellSessionRecord{}, shellBusyError(resolution.SessionRef)
	}
	if data.IsShellSessionNotFound(err) {
		return data.ShellSessionRecord{}, &tools.ExecutionError{ErrorClass: errorSandboxError, Message: "shell session not found", Details: map[string]any{"session_ref": resolution.SessionRef, "code": "shell.session_not_found"}}
	}
	return data.ShellSessionRecord{}, sandboxArgsError(err.Error())
}

func (o *sessionOrchestrator) updateMemoryWriterLease(
	execCtx tools.ExecutionContext,
	resolution *resolvedSession,
	ownerID string,
	leaseUntil time.Time,
	renewOnly bool,
) (data.ShellSessionRecord, *tools.ExecutionError) {
	o.mu.Lock()
	defer o.mu.Unlock()
	record, ok := o.memorySessions[resolution.SessionRef]
	if !ok {
		record = o.sessionRecord(execCtx, resolution)
	}
	now := time.Now().UTC()
	currentOwner := strings.TrimSpace(stringPtrValue(record.LeaseOwnerID))
	active := hasActiveWriterLease(record, now)
	if renewOnly {
		if currentOwner != ownerID {
			return data.ShellSessionRecord{}, shellBusyError(resolution.SessionRef)
		}
	} else if active && currentOwner != ownerID {
		return data.ShellSessionRecord{}, shellBusyError(resolution.SessionRef)
	}
	if currentOwner != "" && currentOwner != ownerID {
		record.LeaseEpoch++
	}
	record.LeaseOwnerID = stringPtr(ownerID)
	record.LeaseUntil = timePtr(leaseUntil)
	record.LastUsedAt = now
	record.UpdatedAt = now
	o.memorySessions[resolution.SessionRef] = record
	return record, nil
}

func (o *sessionOrchestrator) sessionRecord(execCtx tools.ExecutionContext, resolution *resolvedSession) data.ShellSessionRecord {
	if resolution != nil && resolution.Record != nil {
		return *resolution.Record
	}
	return data.ShellSessionRecord{
		SessionRef:        resolution.SessionRef,
		OrgID:             derefUUID(execCtx.OrgID),
		ProfileRef:        strings.TrimSpace(execCtx.ProfileRef),
		WorkspaceRef:      strings.TrimSpace(execCtx.WorkspaceRef),
		ProjectID:         uuidPtr(execCtx.ProjectID),
		ThreadID:          execCtx.ThreadID,
		RunID:             uuidPtr(execCtx.RunID),
		ShareScope:        resolution.ShareScope,
		State:             data.ShellSessionStateReady,
		DefaultBindingKey: resolution.DefaultBindingKey,
		MetadataJSON:      map[string]any{},
	}
}

func (r *resolvedSession) RecordLeaseOwnerID() *string {
	if r == nil || r.Record == nil {
		return nil
	}
	return r.Record.LeaseOwnerID
}

func writerLeaseOwner(execCtx tools.ExecutionContext) string {
	if execCtx.RunID == uuid.Nil {
		return ""
	}
	return "run:" + execCtx.RunID.String()
}

func execWriterLeaseUntil(timeoutMs int) time.Time {
	now := time.Now().UTC()
	leaseTTL := minimumWriterLeaseTTL
	if timeoutMs > 0 {
		candidate := time.Duration(timeoutMs)*time.Millisecond + execWriterLeasePad
		if candidate > leaseTTL {
			leaseTTL = candidate
		}
	}
	return now.Add(leaseTTL)
}

func writeWriterLeaseUntil() time.Time {
	return time.Now().UTC().Add(minimumWriterLeaseTTL)
}

func hasActiveWriterLease(record data.ShellSessionRecord, now time.Time) bool {
	return record.LeaseOwnerID != nil && record.LeaseUntil != nil && record.LeaseUntil.After(now.UTC())
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copyValue := value.UTC()
	return &copyValue
}
