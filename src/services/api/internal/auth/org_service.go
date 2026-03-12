package auth

import (
	"context"
	"fmt"

	"arkloop/services/api/internal/data"
	"arkloop/services/shared/database"

	"github.com/google/uuid"
)

type OrgService struct {
	db             database.DB
	orgRepo        *data.OrgRepository
	membershipRepo *data.OrgMembershipRepository
}

func NewOrgService(db database.DB, orgRepo *data.OrgRepository, membershipRepo *data.OrgMembershipRepository) (*OrgService, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	if orgRepo == nil {
		return nil, fmt.Errorf("orgRepo must not be nil")
	}
	if membershipRepo == nil {
		return nil, fmt.Errorf("membershipRepo must not be nil")
	}
	return &OrgService{db: db, orgRepo: orgRepo, membershipRepo: membershipRepo}, nil
}

type CreateWorkspaceResult struct {
	Org        data.Org
	Membership data.OrgMembership
}

// CreateWorkspace 创建 workspace 类型 org，在事务内将 ownerUserID 设为 owner。
func (s *OrgService) CreateWorkspace(ctx context.Context, slug, name string, ownerUserID uuid.UUID) (CreateWorkspaceResult, error) {
	if slug == "" {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service: slug must not be empty")
	}
	if name == "" {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service: name must not be empty")
	}
	if ownerUserID == uuid.Nil {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service: ownerUserID must not be empty")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service.CreateWorkspace: %w", err)
	}
	defer tx.Rollback(ctx)

	orgRepo, err := data.NewOrgRepository(tx)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}
	membershipRepo, err := data.NewOrgMembershipRepository(tx)
	if err != nil {
		return CreateWorkspaceResult{}, err
	}

	org, err := orgRepo.Create(ctx, slug, name, "workspace")
	if err != nil {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service.CreateWorkspace: create org: %w", err)
	}

	membership, err := membershipRepo.Create(ctx, org.ID, ownerUserID, "owner")
	if err != nil {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service.CreateWorkspace: create membership: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateWorkspaceResult{}, fmt.Errorf("org_service.CreateWorkspace: commit: %w", err)
	}

	return CreateWorkspaceResult{Org: org, Membership: membership}, nil
}
