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
	ID          uuid.UUID
	OrgID       uuid.UUID
	ProjectID   *uuid.UUID
	Scope       string
	ToolName    string
	Description string
	IsDisabled  bool
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

func (r *ToolDescriptionOverridesRepository) ListByScope(ctx context.Context, projectID uuid.UUID, scope string) ([]ToolDescriptionOverride, error) {
	if scope != "project" && scope != "platform" {
		return nil, fmt.Errorf("scope must be project or platform")
	}

	var rows pgx.Rows
	var err error
	if scope == "platform" {
		rows, err = r.db.Query(ctx, `
			SELECT id, org_id, project_id, scope, tool_name, description, is_disabled, updated_at
			FROM tool_description_overrides
			WHERE scope = 'platform'
			ORDER BY tool_name ASC
		`)
	} else {
		rows, err = r.db.Query(ctx, `
			SELECT id, org_id, project_id, scope, tool_name, description, is_disabled, updated_at
			FROM tool_description_overrides
			WHERE project_id = $1 AND scope = 'project'
			ORDER BY tool_name ASC
		`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolDescriptionOverride
	for rows.Next() {
		var o ToolDescriptionOverride
		if err := rows.Scan(&o.ID, &o.OrgID, &o.ProjectID, &o.Scope, &o.ToolName, &o.Description, &o.IsDisabled, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *ToolDescriptionOverridesRepository) Upsert(ctx context.Context, projectID uuid.UUID, scope string, toolName string, description string) error {
	if scope != "project" && scope != "platform" {
		return fmt.Errorf("scope must be project or platform")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return fmt.Errorf("tool_name must not be empty")
	}
	desc := strings.TrimSpace(description)
	if desc == "" {
		return fmt.Errorf("description must not be empty")
	}

	if scope == "platform" {
		_, err := r.db.Exec(ctx, `
			INSERT INTO tool_description_overrides (scope, tool_name, description, updated_at)
			VALUES ('platform', $1, $2, now())
			ON CONFLICT (tool_name) WHERE scope = 'platform'
			DO UPDATE SET description = EXCLUDED.description, updated_at = now()
		`, name, desc)
		return err
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO tool_description_overrides (project_id, scope, tool_name, description, updated_at)
		VALUES ($1, 'project', $2, $3, now())
		ON CONFLICT (project_id, tool_name) WHERE project_id IS NOT NULL
		DO UPDATE SET description = EXCLUDED.description, updated_at = now()
	`, projectID, name, desc)
	return err
}

func (r *ToolDescriptionOverridesRepository) Delete(ctx context.Context, projectID uuid.UUID, scope string, toolName string) error {
	if scope != "project" && scope != "platform" {
		return fmt.Errorf("scope must be project or platform")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return fmt.Errorf("tool_name must not be empty")
	}

	if scope == "platform" {
		tag, err := r.db.Exec(ctx, `
			UPDATE tool_description_overrides
			SET description = '', updated_at = now()
			WHERE scope = 'platform' AND tool_name = $1 AND is_disabled = TRUE
		`, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}

		tag, err = r.db.Exec(ctx, `
			DELETE FROM tool_description_overrides
			WHERE scope = 'platform' AND tool_name = $1
		`, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	}

	tag, err := r.db.Exec(ctx, `
		UPDATE tool_description_overrides
		SET description = '', updated_at = now()
		WHERE project_id = $1 AND scope = 'project' AND tool_name = $2 AND is_disabled = TRUE
	`, projectID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	tag, err = r.db.Exec(ctx, `
		DELETE FROM tool_description_overrides
		WHERE project_id = $1 AND scope = 'project' AND tool_name = $2
	`, projectID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *ToolDescriptionOverridesRepository) SetDisabled(ctx context.Context, projectID uuid.UUID, scope string, toolName string, disabled bool) error {
	if scope != "project" && scope != "platform" {
		return fmt.Errorf("scope must be project or platform")
	}
	name := strings.TrimSpace(toolName)
	if name == "" {
		return fmt.Errorf("tool_name must not be empty")
	}

	if disabled {
		if scope == "platform" {
			_, err := r.db.Exec(ctx, `
				INSERT INTO tool_description_overrides (scope, tool_name, description, is_disabled, updated_at)
				VALUES ('platform', $1, '', TRUE, now())
				ON CONFLICT (tool_name) WHERE scope = 'platform'
				DO UPDATE SET is_disabled = TRUE, updated_at = now()
			`, name)
			return err
		}
		_, err := r.db.Exec(ctx, `
			INSERT INTO tool_description_overrides (project_id, scope, tool_name, description, is_disabled, updated_at)
			VALUES ($1, 'project', $2, '', TRUE, now())
			ON CONFLICT (project_id, tool_name) WHERE project_id IS NOT NULL
			DO UPDATE SET is_disabled = TRUE, updated_at = now()
		`, projectID, name)
		return err
	}

	if scope == "platform" {
		tag, err := r.db.Exec(ctx, `
			UPDATE tool_description_overrides
			SET is_disabled = FALSE, updated_at = now()
			WHERE scope = 'platform' AND tool_name = $1 AND description <> ''
		`, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
		_, err = r.db.Exec(ctx, `
			DELETE FROM tool_description_overrides
			WHERE scope = 'platform' AND tool_name = $1
		`, name)
		return err
	}

	tag, err := r.db.Exec(ctx, `
		UPDATE tool_description_overrides
		SET is_disabled = FALSE, updated_at = now()
		WHERE project_id = $1 AND scope = 'project' AND tool_name = $2 AND description <> ''
	`, projectID, name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	_, err = r.db.Exec(ctx, `
		DELETE FROM tool_description_overrides
		WHERE project_id = $1 AND scope = 'project' AND tool_name = $2
	`, projectID, name)
	return err
}
