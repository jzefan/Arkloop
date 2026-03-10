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

const (
	PersonaScopeOrg      = "org"
	PersonaScopePlatform = "platform"
)

func (r *PersonasRepository) WithTx(tx pgx.Tx) *PersonasRepository {
	return &PersonasRepository{db: tx}
}

type PersonaConflictError struct {
	PersonaKey string
	Version    string
}

func (e PersonaConflictError) Error() string {
	return fmt.Sprintf("persona %q@%q already exists", e.PersonaKey, e.Version)
}

type Persona struct {
	ID                  uuid.UUID
	OrgID               *uuid.UUID
	PersonaKey          string
	Version             string
	DisplayName         string
	Description         *string
	PromptMD            string
	ToolAllowlist       []string
	ToolDenylist        []string
	BudgetsJSON         json.RawMessage
	IsActive            bool
	CreatedAt           time.Time
	PreferredCredential *string
	Model               *string
	ReasoningMode       string
	PromptCacheControl  string
	ExecutorType        string
	ExecutorConfigJSON  json.RawMessage
}

type PersonaPatch struct {
	DisplayName         *string
	Description         *string
	PromptMD            *string
	ToolAllowlist       []string
	ToolDenylist        []string
	BudgetsJSON         json.RawMessage
	IsActive            *bool
	PreferredCredential *string
	Model               *string
	ReasoningMode       *string
	PromptCacheControl  *string
	ExecutorType        *string
	ExecutorConfigJSON  json.RawMessage
}

type PersonasRepository struct {
	db Querier
}

func NewPersonasRepository(db Querier) (*PersonasRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &PersonasRepository{db: db}, nil
}

func NormalizePersonaScope(scope string) (string, error) {
	switch strings.TrimSpace(scope) {
	case PersonaScopeOrg:
		return PersonaScopeOrg, nil
	case PersonaScopePlatform:
		return PersonaScopePlatform, nil
	default:
		return "", fmt.Errorf("scope must be org or platform")
	}
}

func (r *PersonasRepository) Create(
	ctx context.Context,
	orgID uuid.UUID,
	personaKey string,
	version string,
	displayName string,
	description *string,
	promptMD string,
	toolAllowlist []string,
	toolDenylist []string,
	budgetsJSON json.RawMessage,
	preferredCredential *string,
	model *string,
	reasoningMode string,
	promptCacheControl string,
	executorType string,
	executorConfigJSON json.RawMessage,
) (Persona, error) {
	if orgID == uuid.Nil {
		return Persona{}, fmt.Errorf("org_id must not be nil")
	}
	orgIDCopy := orgID
	return r.createWithOrgID(
		ctx,
		&orgIDCopy,
		personaKey,
		version,
		displayName,
		description,
		promptMD,
		toolAllowlist,
		toolDenylist,
		budgetsJSON,
		preferredCredential,
		model,
		reasoningMode,
		promptCacheControl,
		executorType,
		executorConfigJSON,
	)
}

func (r *PersonasRepository) CreateInScope(
	ctx context.Context,
	orgID uuid.UUID,
	scope string,
	personaKey string,
	version string,
	displayName string,
	description *string,
	promptMD string,
	toolAllowlist []string,
	toolDenylist []string,
	budgetsJSON json.RawMessage,
	preferredCredential *string,
	model *string,
	reasoningMode string,
	promptCacheControl string,
	executorType string,
	executorConfigJSON json.RawMessage,
) (Persona, error) {
	normalized, err := NormalizePersonaScope(scope)
	if err != nil {
		return Persona{}, err
	}
	var orgIDPtr *uuid.UUID
	if normalized == PersonaScopeOrg {
		if orgID == uuid.Nil {
			return Persona{}, fmt.Errorf("org_id must not be nil")
		}
		orgIDCopy := orgID
		orgIDPtr = &orgIDCopy
	}
	return r.createWithOrgID(
		ctx,
		orgIDPtr,
		personaKey,
		version,
		displayName,
		description,
		promptMD,
		toolAllowlist,
		toolDenylist,
		budgetsJSON,
		preferredCredential,
		model,
		reasoningMode,
		promptCacheControl,
		executorType,
		executorConfigJSON,
	)
}

func (r *PersonasRepository) createWithOrgID(
	ctx context.Context,
	orgID *uuid.UUID,
	personaKey string,
	version string,
	displayName string,
	description *string,
	promptMD string,
	toolAllowlist []string,
	toolDenylist []string,
	budgetsJSON json.RawMessage,
	preferredCredential *string,
	model *string,
	reasoningMode string,
	promptCacheControl string,
	executorType string,
	executorConfigJSON json.RawMessage,
) (Persona, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(personaKey) == "" {
		return Persona{}, fmt.Errorf("persona_key must not be empty")
	}
	if strings.TrimSpace(version) == "" {
		return Persona{}, fmt.Errorf("version must not be empty")
	}
	if strings.TrimSpace(displayName) == "" {
		return Persona{}, fmt.Errorf("display_name must not be empty")
	}
	if strings.TrimSpace(promptMD) == "" {
		return Persona{}, fmt.Errorf("prompt_md must not be empty")
	}

	if len(budgetsJSON) == 0 {
		budgetsJSON = json.RawMessage("{}")
	}
	if toolAllowlist == nil {
		toolAllowlist = []string{}
	}
	if toolDenylist == nil {
		toolDenylist = []string{}
	}
	preferredCredential = normalizeOptionalPersonaString(preferredCredential)
	model = normalizeOptionalPersonaString(model)
	reasoningMode = normalizePersonaReasoningMode(reasoningMode)
	promptCacheControl = normalizePersonaPromptCacheControl(promptCacheControl)
	if strings.TrimSpace(executorType) == "" {
		executorType = "agent.simple"
	}
	if len(executorConfigJSON) == 0 {
		executorConfigJSON = json.RawMessage("{}")
	}

	var persona Persona
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO personas
		    (org_id, persona_key, version, display_name, description, prompt_md,
		     tool_allowlist, tool_denylist, budgets_json, preferred_credential,
		     model, reasoning_mode, prompt_cache_control,
		     executor_type, executor_config_json)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING id, org_id, persona_key, version, display_name, description,
		           prompt_md, tool_allowlist, tool_denylist, budgets_json, is_active, created_at,
		           preferred_credential, model, reasoning_mode, prompt_cache_control,
		           executor_type, executor_config_json`,
		orgID, personaKey, version, displayName, description, promptMD,
		toolAllowlist, toolDenylist, budgetsJSON, preferredCredential,
		model, reasoningMode, promptCacheControl,
		executorType, executorConfigJSON,
	).Scan(
		&persona.ID, &persona.OrgID, &persona.PersonaKey, &persona.Version,
		&persona.DisplayName, &persona.Description, &persona.PromptMD,
		&persona.ToolAllowlist, &persona.ToolDenylist, &persona.BudgetsJSON, &persona.IsActive, &persona.CreatedAt,
		&persona.PreferredCredential, &persona.Model, &persona.ReasoningMode, &persona.PromptCacheControl,
		&persona.ExecutorType, &persona.ExecutorConfigJSON,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Persona{}, PersonaConflictError{PersonaKey: personaKey, Version: version}
		}
		return Persona{}, err
	}
	return persona, nil
}

func (r *PersonasRepository) GetByID(ctx context.Context, orgID, id uuid.UUID) (*Persona, error) {
	return r.GetByIDInScope(ctx, orgID, id, PersonaScopeOrg)
}

func (r *PersonasRepository) GetByIDInScope(ctx context.Context, orgID, id uuid.UUID, scope string) (*Persona, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	whereClause, scopeArgs, err := personaScopeWhereClause(scope, orgID, 2)
	if err != nil {
		return nil, err
	}

	var persona Persona
	args := append([]any{id}, scopeArgs...)
	err = r.db.QueryRow(
		ctx,
		fmt.Sprintf(`SELECT id, org_id, persona_key, version, display_name, description,
		        prompt_md, tool_allowlist, tool_denylist, budgets_json, is_active, created_at,
		        preferred_credential, model, reasoning_mode, prompt_cache_control,
		        executor_type, executor_config_json
		 FROM personas
		 WHERE id = $1 AND %s`, whereClause),
		args...,
	).Scan(
		&persona.ID, &persona.OrgID, &persona.PersonaKey, &persona.Version,
		&persona.DisplayName, &persona.Description, &persona.PromptMD,
		&persona.ToolAllowlist, &persona.ToolDenylist, &persona.BudgetsJSON, &persona.IsActive, &persona.CreatedAt,
		&persona.PreferredCredential, &persona.Model, &persona.ReasoningMode, &persona.PromptCacheControl,
		&persona.ExecutorType, &persona.ExecutorConfigJSON,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &persona, nil
}

func (r *PersonasRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Persona, error) {
	return r.ListByScope(ctx, orgID, PersonaScopeOrg)
}

func (r *PersonasRepository) ListByScope(ctx context.Context, orgID uuid.UUID, scope string) ([]Persona, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	whereClause, scopeArgs, err := personaScopeWhereClause(scope, orgID, 1)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(
		ctx,
		fmt.Sprintf(`SELECT id, org_id, persona_key, version, display_name, description,
		        prompt_md, tool_allowlist, tool_denylist, budgets_json, is_active, created_at,
		        preferred_credential, model, reasoning_mode, prompt_cache_control,
		        executor_type, executor_config_json
		 FROM personas
		 WHERE %s
		 ORDER BY created_at ASC`, whereClause),
		scopeArgs...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPersonas(rows)
}

func (r *PersonasRepository) ListActiveByOrg(ctx context.Context, orgID uuid.UUID) ([]Persona, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT id, org_id, persona_key, version, display_name, description,
		        prompt_md, tool_allowlist, tool_denylist, budgets_json, is_active, created_at,
		        preferred_credential, model, reasoning_mode, prompt_cache_control,
		        executor_type, executor_config_json
		 FROM personas
		 WHERE org_id = $1 AND is_active = TRUE
		 ORDER BY created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPersonas(rows)
}

func (r *PersonasRepository) ListActiveEffective(ctx context.Context, orgID uuid.UUID) ([]Persona, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT id, org_id, persona_key, version, display_name, description,
		        prompt_md, tool_allowlist, tool_denylist, budgets_json, is_active, created_at,
		        preferred_credential, model, reasoning_mode, prompt_cache_control,
		        executor_type, executor_config_json
		 FROM personas
		 WHERE is_active = TRUE AND (org_id IS NULL OR org_id = $1)
		 ORDER BY CASE WHEN org_id IS NULL THEN 0 ELSE 1 END ASC, created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPersonas(rows)
}

func (r *PersonasRepository) Patch(ctx context.Context, orgID, id uuid.UUID, patch PersonaPatch) (*Persona, error) {
	return r.PatchInScope(ctx, orgID, id, PersonaScopeOrg, patch)
}

func (r *PersonasRepository) PatchInScope(ctx context.Context, orgID, id uuid.UUID, scope string, patch PersonaPatch) (*Persona, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	setClauses := make([]string, 0, 12)
	args := make([]any, 0, 12)
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
	if patch.ToolDenylist != nil {
		setClauses = append(setClauses, fmt.Sprintf("tool_denylist = $%d", argIdx))
		args = append(args, patch.ToolDenylist)
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
	if patch.PreferredCredential != nil {
		value := normalizeOptionalPersonaString(patch.PreferredCredential)
		if value == nil {
			setClauses = append(setClauses, "preferred_credential = NULL")
		} else {
			setClauses = append(setClauses, fmt.Sprintf("preferred_credential = $%d", argIdx))
			args = append(args, *value)
			argIdx++
		}
	}
	if patch.Model != nil {
		value := normalizeOptionalPersonaString(patch.Model)
		if value == nil {
			setClauses = append(setClauses, "model = NULL")
		} else {
			setClauses = append(setClauses, fmt.Sprintf("model = $%d", argIdx))
			args = append(args, *value)
			argIdx++
		}
	}
	if patch.ReasoningMode != nil {
		setClauses = append(setClauses, fmt.Sprintf("reasoning_mode = $%d", argIdx))
		args = append(args, normalizePersonaReasoningMode(*patch.ReasoningMode))
		argIdx++
	}
	if patch.PromptCacheControl != nil {
		setClauses = append(setClauses, fmt.Sprintf("prompt_cache_control = $%d", argIdx))
		args = append(args, normalizePersonaPromptCacheControl(*patch.PromptCacheControl))
		argIdx++
	}
	if patch.ExecutorType != nil {
		trimmed := strings.TrimSpace(*patch.ExecutorType)
		if trimmed == "" {
			trimmed = "agent.simple"
		}
		setClauses = append(setClauses, fmt.Sprintf("executor_type = $%d", argIdx))
		args = append(args, trimmed)
		argIdx++
	}
	if len(patch.ExecutorConfigJSON) > 0 {
		setClauses = append(setClauses, fmt.Sprintf("executor_config_json = $%d", argIdx))
		args = append(args, patch.ExecutorConfigJSON)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetByIDInScope(ctx, orgID, id, scope)
	}

	whereClause, scopeArgs, err := personaScopeWhereClause(scope, orgID, argIdx+1)
	if err != nil {
		return nil, err
	}
	args = append(args, id)
	args = append(args, scopeArgs...)
	idIdx := argIdx

	var persona Persona
	err = r.db.QueryRow(
		ctx,
		fmt.Sprintf(`UPDATE personas
		 SET %s
		 WHERE id = $%d AND %s
		 RETURNING id, org_id, persona_key, version, display_name, description,
		           prompt_md, tool_allowlist, tool_denylist, budgets_json, is_active, created_at,
		           preferred_credential, model, reasoning_mode, prompt_cache_control,
		           executor_type, executor_config_json`,
			strings.Join(setClauses, ", "), idIdx, whereClause),
		args...,
	).Scan(
		&persona.ID, &persona.OrgID, &persona.PersonaKey, &persona.Version,
		&persona.DisplayName, &persona.Description, &persona.PromptMD,
		&persona.ToolAllowlist, &persona.ToolDenylist, &persona.BudgetsJSON, &persona.IsActive, &persona.CreatedAt,
		&persona.PreferredCredential, &persona.Model, &persona.ReasoningMode, &persona.PromptCacheControl,
		&persona.ExecutorType, &persona.ExecutorConfigJSON,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if isUniqueViolation(err) {
			return nil, PersonaConflictError{}
		}
		return nil, err
	}
	return &persona, nil
}

func scanPersonas(rows pgx.Rows) ([]Persona, error) {
	personas := []Persona{}
	for rows.Next() {
		var s Persona
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.PersonaKey, &s.Version,
			&s.DisplayName, &s.Description, &s.PromptMD,
			&s.ToolAllowlist, &s.ToolDenylist, &s.BudgetsJSON, &s.IsActive, &s.CreatedAt,
			&s.PreferredCredential, &s.Model, &s.ReasoningMode, &s.PromptCacheControl,
			&s.ExecutorType, &s.ExecutorConfigJSON,
		); err != nil {
			return nil, err
		}
		personas = append(personas, s)
	}
	return personas, rows.Err()
}

func (r *PersonasRepository) Delete(ctx context.Context, orgID, id uuid.UUID) (bool, error) {
	return r.DeleteInScope(ctx, orgID, id, PersonaScopeOrg)
}

func (r *PersonasRepository) DeleteInScope(ctx context.Context, orgID, id uuid.UUID, scope string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	whereClause, scopeArgs, err := personaScopeWhereClause(scope, orgID, 2)
	if err != nil {
		return false, err
	}
	args := append([]any{id}, scopeArgs...)
	tag, err := r.db.Exec(
		ctx,
		fmt.Sprintf(`DELETE FROM personas WHERE id = $1 AND %s`, whereClause),
		args...,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func personaScopeWhereClause(scope string, orgID uuid.UUID, argIdx int) (string, []any, error) {
	normalized, err := NormalizePersonaScope(scope)
	if err != nil {
		return "", nil, err
	}
	if normalized == PersonaScopePlatform {
		return "org_id IS NULL", nil, nil
	}
	if orgID == uuid.Nil {
		return "", nil, fmt.Errorf("org_id must not be nil")
	}
	return fmt.Sprintf("org_id = $%d", argIdx), []any{orgID}, nil
}

func normalizeOptionalPersonaString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizePersonaReasoningMode(value string) string {
	switch strings.TrimSpace(value) {
	case "enabled", "disabled", "none", "auto", "low", "medium", "high":
		return strings.TrimSpace(value)
	default:
		return "auto"
	}
}

func normalizePersonaPromptCacheControl(value string) string {
	switch strings.TrimSpace(value) {
	case "system_prompt", "none":
		return strings.TrimSpace(value)
	default:
		return "none"
	}
}
