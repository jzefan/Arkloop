package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *SkillsRepository) WithTx(tx pgx.Tx) *SkillsRepository {
	return &SkillsRepository{db: tx}
}

type SkillConflictError struct {
	SkillKey string
	Version  string
}

func (e SkillConflictError) Error() string {
	return fmt.Sprintf("skill %q@%q already exists", e.SkillKey, e.Version)
}

type Skill struct {
	ID            uuid.UUID
	OrgID         *uuid.UUID
	SkillKey      string
	Version       string
	DisplayName   string
	Description   *string
	PromptMD      string
	ToolAllowlist []string
	BudgetsJSON   json.RawMessage
	IsActive      bool
	CreatedAt     time.Time
}

type SkillPatch struct {
	DisplayName   *string
	Description   *string
	PromptMD      *string
	ToolAllowlist []string
	BudgetsJSON   json.RawMessage
	IsActive      *bool
}

type SkillsRepository struct {
	db Querier
}

func NewSkillsRepository(db Querier) (*SkillsRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &SkillsRepository{db: db}, nil
}

func (r *SkillsRepository) Create(
	ctx context.Context,
	orgID uuid.UUID,
	skillKey string,
	version string,
	displayName string,
	description *string,
	promptMD string,
	toolAllowlist []string,
	budgetsJSON json.RawMessage,
) (Skill, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return Skill{}, fmt.Errorf("org_id must not be nil")
	}
	if strings.TrimSpace(skillKey) == "" {
		return Skill{}, fmt.Errorf("skill_key must not be empty")
	}
	if strings.TrimSpace(version) == "" {
		return Skill{}, fmt.Errorf("version must not be empty")
	}
	if strings.TrimSpace(displayName) == "" {
		return Skill{}, fmt.Errorf("display_name must not be empty")
	}
	if strings.TrimSpace(promptMD) == "" {
		return Skill{}, fmt.Errorf("prompt_md must not be empty")
	}

	if len(budgetsJSON) == 0 {
		budgetsJSON = json.RawMessage("{}")
	}
	if toolAllowlist == nil {
		toolAllowlist = []string{}
	}

	var skill Skill
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO skills
		    (org_id, skill_key, version, display_name, description, prompt_md,
		     tool_allowlist, budgets_json)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, org_id, skill_key, version, display_name, description,
		           prompt_md, tool_allowlist, budgets_json, is_active, created_at`,
		orgID, skillKey, version, displayName, description, promptMD,
		toolAllowlist, budgetsJSON,
	).Scan(
		&skill.ID, &skill.OrgID, &skill.SkillKey, &skill.Version,
		&skill.DisplayName, &skill.Description, &skill.PromptMD,
		&skill.ToolAllowlist, &skill.BudgetsJSON, &skill.IsActive, &skill.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Skill{}, SkillConflictError{SkillKey: skillKey, Version: version}
		}
		return Skill{}, err
	}
	return skill, nil
}

func (r *SkillsRepository) GetByID(ctx context.Context, orgID, id uuid.UUID) (*Skill, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var skill Skill
	err := r.db.QueryRow(
		ctx,
		`SELECT id, org_id, skill_key, version, display_name, description,
		        prompt_md, tool_allowlist, budgets_json, is_active, created_at
		 FROM skills
		 WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(
		&skill.ID, &skill.OrgID, &skill.SkillKey, &skill.Version,
		&skill.DisplayName, &skill.Description, &skill.PromptMD,
		&skill.ToolAllowlist, &skill.BudgetsJSON, &skill.IsActive, &skill.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &skill, nil
}

// ListByOrg 返回该 org 的所有 skill（含 org_id IS NULL 的全局 skill）。
func (r *SkillsRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Skill, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT id, org_id, skill_key, version, display_name, description,
		        prompt_md, tool_allowlist, budgets_json, is_active, created_at
		 FROM skills
		 WHERE org_id = $1 OR org_id IS NULL
		 ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSkills(rows)
}

// ListActiveByOrg 仅返回该 org 的 is_active=true 的 skill，供 Worker 执行时使用。
// 不包含全局（org_id IS NULL）skill，全局 skill 由文件系统负责。
func (r *SkillsRepository) ListActiveByOrg(ctx context.Context, orgID uuid.UUID) ([]Skill, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT id, org_id, skill_key, version, display_name, description,
		        prompt_md, tool_allowlist, budgets_json, is_active, created_at
		 FROM skills
		 WHERE org_id = $1 AND is_active = TRUE
		 ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSkills(rows)
}

func (r *SkillsRepository) Patch(ctx context.Context, orgID, id uuid.UUID, patch SkillPatch) (*Skill, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if patch.DisplayName != nil {
		trimmed := strings.TrimSpace(*patch.DisplayName)
		if trimmed == "" {
			return nil, fmt.Errorf("display_name must not be empty")
		}
		setClauses = append(setClauses, fmt.Sprintf("display_name = $%d", argIdx))
		args = append(args, trimmed)
		argIdx++
	}
	if patch.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *patch.Description)
		argIdx++
	}
	if patch.PromptMD != nil {
		trimmed := strings.TrimSpace(*patch.PromptMD)
		if trimmed == "" {
			return nil, fmt.Errorf("prompt_md must not be empty")
		}
		setClauses = append(setClauses, fmt.Sprintf("prompt_md = $%d", argIdx))
		args = append(args, trimmed)
		argIdx++
	}
	if patch.ToolAllowlist != nil {
		setClauses = append(setClauses, fmt.Sprintf("tool_allowlist = $%d", argIdx))
		args = append(args, patch.ToolAllowlist)
		argIdx++
	}
	if len(patch.BudgetsJSON) > 0 {
		setClauses = append(setClauses, fmt.Sprintf("budgets_json = $%d", argIdx))
		args = append(args, patch.BudgetsJSON)
		argIdx++
	}
	if patch.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *patch.IsActive)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetByID(ctx, orgID, id)
	}

	args = append(args, id, orgID)
	idIdx := argIdx
	orgIdx := argIdx + 1

	var skill Skill
	err := r.db.QueryRow(
		ctx,
		fmt.Sprintf(`UPDATE skills
		 SET %s
		 WHERE id = $%d AND org_id = $%d
		 RETURNING id, org_id, skill_key, version, display_name, description,
		           prompt_md, tool_allowlist, budgets_json, is_active, created_at`,
			strings.Join(setClauses, ", "), idIdx, orgIdx),
		args...,
	).Scan(
		&skill.ID, &skill.OrgID, &skill.SkillKey, &skill.Version,
		&skill.DisplayName, &skill.Description, &skill.PromptMD,
		&skill.ToolAllowlist, &skill.BudgetsJSON, &skill.IsActive, &skill.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if isUniqueViolation(err) {
			return nil, SkillConflictError{}
		}
		return nil, err
	}
	return &skill, nil
}

func scanSkills(rows pgx.Rows) ([]Skill, error) {
	skills := []Skill{}
	for rows.Next() {
		var s Skill
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.SkillKey, &s.Version,
			&s.DisplayName, &s.Description, &s.PromptMD,
			&s.ToolAllowlist, &s.BudgetsJSON, &s.IsActive, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	return skills, rows.Err()
}
