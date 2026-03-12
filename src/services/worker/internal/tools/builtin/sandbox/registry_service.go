package sandbox

import (
	"context"
	"strings"
	"time"

	"arkloop/services/shared/database"
	"arkloop/services/worker/internal/data"
	"github.com/google/uuid"
)

type registryService struct {
	db            database.DB
	profileRepo   data.ProfileRegistriesRepository
	workspaceRepo data.WorkspaceRegistriesRepository
	sessionsRepo  data.ShellSessionsRepository
}

func newRegistryService(db database.DB) *registryService {
	return &registryService{db: db}
}

func (s *registryService) UpsertProfileRegistry(
	ctx context.Context,
	orgID uuid.UUID,
	ownerUserID *uuid.UUID,
	profileRef string,
	defaultWorkspaceRef *string,
) error {
	if s == nil || s.db == nil {
		return nil
	}
	profileRef = strings.TrimSpace(profileRef)
	if orgID == uuid.Nil || profileRef == "" {
		return nil
	}
	return s.profileRepo.UpsertTouch(ctx, s.db, data.RegistryRecord{
		Ref:                 profileRef,
		OrgID:               orgID,
		OwnerUserID:         ownerUserID,
		DefaultWorkspaceRef: defaultWorkspaceRef,
		FlushState:          data.FlushStateIdle,
		LastUsedAt:          time.Now().UTC(),
		MetadataJSON:        map[string]any{},
	})
}

func (s *registryService) UpsertWorkspaceRegistry(
	ctx context.Context,
	orgID uuid.UUID,
	ownerUserID *uuid.UUID,
	projectID *uuid.UUID,
	workspaceRef string,
	defaultShellSessionRef *string,
) error {
	if s == nil || s.db == nil {
		return nil
	}
	workspaceRef = strings.TrimSpace(workspaceRef)
	if orgID == uuid.Nil || workspaceRef == "" {
		return nil
	}
	return s.workspaceRepo.UpsertTouch(ctx, s.db, data.RegistryRecord{
		Ref:                    workspaceRef,
		OrgID:                  orgID,
		OwnerUserID:            ownerUserID,
		ProjectID:              projectID,
		DefaultShellSessionRef: defaultShellSessionRef,
		FlushState:             data.FlushStateIdle,
		LastUsedAt:             time.Now().UTC(),
		MetadataJSON:           map[string]any{},
	})
}

func (s *registryService) BindSessionRestorePointer(ctx context.Context, orgID uuid.UUID, sessionRef string, revision string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if orgID == uuid.Nil || strings.TrimSpace(sessionRef) == "" {
		return nil
	}
	return s.sessionsRepo.UpdateRestoreRevision(ctx, s.db, orgID, sessionRef, revision)
}
