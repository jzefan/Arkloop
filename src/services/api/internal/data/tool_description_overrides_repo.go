package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ToolDescriptionOverride struct {
	OrgID       uuid.UUID
	Scope       string
	ToolName    string
	Description string
	UpdatedAt   time.Time
}

type ToolDescriptionOverridesRepository struct {
	db Querier
}

func NewToolDescriptionOverridesRepository(db Querier) (*ToolDescriptionOverridesRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	return &ToolDescriptionOverridesRepository{db: db}, nil
}

func (r *ToolDescriptionOverridesRepository) ListByScope(ctx context.Context, orgID uuid.UUID, scope string) ([]ToolDescriptionOverride, error) {
	if scope != "org" && scope != "platform" {
		return nil, fmt.Errorf("scope must be org or platform")
	}

	rows, err := r.db.Query(ctx, `
		SELECT org_id, scope, tool_name, description, updated_at
		FROM tool_description_overrides
		WHERE org_id = $1 AND scope = $2
		ORDER BY tool_name ASC
	`, orgID, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolDescriptionOverride
	for rows.Next() {
		var o ToolDescriptionOverride
		if err := rows.Scan(&o.OrgID, &o.Scope, &o.ToolName, &o.Description, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *ToolDescriptionOverridesRepository) Upsert(ctx context.Context, orgID uuid.UUID, scope string, toolName string, description string) error {
	if scope != "org" && scope != "platform" {
		return fmt.Errorf("scope must be org or platform")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return fmt.Errorf("tool_name must not be empty")
	}
	desc := strings.TrimSpace(description)
	if desc == "" {
		return fmt.Errorf("description must not be empty")
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO tool_description_overrides (org_id, scope, tool_name, description, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (org_id, scope, tool_name)
		DO UPDATE SET description = EXCLUDED.description, updated_at = now()
	`, orgID, scope, name, desc)
	return err
}

func (r *ToolDescriptionOverridesRepository) Delete(ctx context.Context, orgID uuid.UUID, scope string, toolName string) error {
	if scope != "org" && scope != "platform" {
		return fmt.Errorf("scope must be org or platform")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return fmt.Errorf("tool_name must not be empty")
	}

	tag, err := r.db.Exec(ctx, `
		DELETE FROM tool_description_overrides
		WHERE org_id = $1 AND scope = $2 AND tool_name = $3
	`, orgID, scope, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
