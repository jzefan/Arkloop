package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type WorkspaceSkillEnablement struct {
	WorkspaceRef    string
	OrgID           uuid.UUID
	EnabledByUserID uuid.UUID
	SkillKey        string
	Version         string
	DisplayName     string
	Description     *string
	InstructionPath string
	ManifestKey     string
	BundleKey       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WorkspaceSkillEnablementsRepository struct {
	db Querier
}

func NewWorkspaceSkillEnablementsRepository(db Querier) (*WorkspaceSkillEnablementsRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &WorkspaceSkillEnablementsRepository{db: db}, nil
}

func (r *WorkspaceSkillEnablementsRepository) Replace(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, workspaceRef string, enabledByUserID uuid.UUID, items []WorkspaceSkillEnablement) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if tx == nil {
		return fmt.Errorf("tx must not be nil")
	}
	workspaceRef = strings.TrimSpace(workspaceRef)
	if orgID == uuid.Nil || enabledByUserID == uuid.Nil || workspaceRef == "" {
		return fmt.Errorf("workspace enablement is invalid")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM workspace_skill_enablements WHERE org_id = $1 AND workspace_ref = $2`, orgID, workspaceRef); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO workspace_skill_enablements (workspace_ref, org_id, enabled_by_user_id, skill_key, version)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (workspace_ref, skill_key) DO UPDATE
			 SET version = EXCLUDED.version,
			     enabled_by_user_id = EXCLUDED.enabled_by_user_id,
			     updated_at = now()`,
			workspaceRef,
			orgID,
			enabledByUserID,
			strings.TrimSpace(item.SkillKey),
			strings.TrimSpace(item.Version),
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *WorkspaceSkillEnablementsRepository) ListByWorkspace(ctx context.Context, orgID uuid.UUID, workspaceRef string) ([]WorkspaceSkillEnablement, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := r.db.Query(
		ctx,
		`SELECT wse.workspace_ref, wse.org_id, wse.enabled_by_user_id, wse.skill_key, wse.version, sp.display_name, sp.description, sp.instruction_path, sp.manifest_key, sp.bundle_key, wse.created_at, wse.updated_at
		   FROM workspace_skill_enablements wse
		   JOIN skill_packages sp ON sp.org_id = wse.org_id AND sp.skill_key = wse.skill_key AND sp.version = wse.version
		  WHERE wse.org_id = $1 AND wse.workspace_ref = $2
		  ORDER BY wse.skill_key`,
		orgID,
		strings.TrimSpace(workspaceRef),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkspaceSkillEnablement, 0)
	for rows.Next() {
		var item WorkspaceSkillEnablement
		if err := rows.Scan(&item.WorkspaceRef, &item.OrgID, &item.EnabledByUserID, &item.SkillKey, &item.Version, &item.DisplayName, &item.Description, &item.InstructionPath, &item.ManifestKey, &item.BundleKey, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
