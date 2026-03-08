package sandbox

import (
	"context"
	"strings"

	"arkloop/services/worker/internal/data"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type registryService struct {
	pool          *pgxpool.Pool
	profileRepo   data.ProfileRegistriesRepository
	workspaceRepo data.WorkspaceRegistriesRepository
	sessionsRepo  data.ShellSessionsRepository
}

func newRegistryService(pool *pgxpool.Pool) *registryService {
	return &registryService{pool: pool}
}

func (s *registryService) EnsureProfileRegistry(ctx context.Context, orgID uuid.UUID, profileRef string) error {
	if s == nil || s.pool == nil {
		return nil
	}
	profileRef = strings.TrimSpace(profileRef)
	if orgID == uuid.Nil || profileRef == "" {
		return nil
	}
	_, err := s.profileRepo.GetOrCreate(ctx, s.pool, data.RegistryRecord{
		Ref:          profileRef,
		OrgID:        orgID,
		FlushState:   data.FlushStateIdle,
		MetadataJSON: map[string]any{},
	})
	return err
}

func (s *registryService) EnsureWorkspaceRegistry(ctx context.Context, orgID uuid.UUID, workspaceRef string) error {
	if s == nil || s.pool == nil {
		return nil
	}
	workspaceRef = strings.TrimSpace(workspaceRef)
	if orgID == uuid.Nil || workspaceRef == "" {
		return nil
	}
	_, err := s.workspaceRepo.GetOrCreate(ctx, s.pool, data.RegistryRecord{
		Ref:          workspaceRef,
		OrgID:        orgID,
		FlushState:   data.FlushStateIdle,
		MetadataJSON: map[string]any{},
	})
	return err
}

func (s *registryService) BindSessionRestorePointer(ctx context.Context, orgID uuid.UUID, sessionRef string, revision string) error {
	if s == nil || s.pool == nil {
		return nil
	}
	if orgID == uuid.Nil || strings.TrimSpace(sessionRef) == "" {
		return nil
	}
	return s.sessionsRepo.UpdateRestoreRevision(ctx, s.pool, orgID, sessionRef, revision)
}
