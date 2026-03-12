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

// WithTx 返回一个使用给定事务的 ToolProviderConfigsRepository 副本。
func (r *ToolProviderConfigsRepository) WithTx(tx pgx.Tx) *ToolProviderConfigsRepository {
	return &ToolProviderConfigsRepository{db: tx}
}

type ToolProviderConfig struct {
	ID           uuid.UUID
	OrgID        *uuid.UUID // legacy, kept for backward compat
	Scope        string     // "project" | "platform"
	ProjectID    *uuid.UUID
	GroupName    string
	ProviderName string
	IsActive     bool
	SecretID     *uuid.UUID
	KeyPrefix    *string
	BaseURL      *string
	ConfigJSON   json.RawMessage
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ToolProviderConfigsRepository struct {
	db Querier
}

func NewToolProviderConfigsRepository(db Querier) (*ToolProviderConfigsRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &ToolProviderConfigsRepository{db: db}, nil
}

func (r *ToolProviderConfigsRepository) ListByScope(ctx context.Context, projectID uuid.UUID, scope string) ([]ToolProviderConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if scope != "project" && scope != "platform" {
		return nil, fmt.Errorf("scope must be project or platform")
	}
	if scope == "project" && projectID == uuid.Nil {
		return nil, fmt.Errorf("project_id must not be empty for project scope")
	}

	var rows pgx.Rows
	var err error
	if scope == "platform" {
		rows, err = r.db.Query(ctx, `
			SELECT id, org_id, scope, project_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
			FROM tool_provider_configs
			WHERE scope = 'platform'
			ORDER BY group_name ASC, provider_name ASC
		`)
	} else {
		rows, err = r.db.Query(ctx, `
			SELECT id, org_id, scope, project_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
			FROM tool_provider_configs
			WHERE project_id = $1 AND scope = 'project'
			ORDER BY group_name ASC, provider_name ASC
		`, projectID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ToolProviderConfig{}
	for rows.Next() {
		var cfg ToolProviderConfig
		if err := rows.Scan(
			&cfg.ID,
			&cfg.OrgID,
			&cfg.Scope,
			&cfg.ProjectID,
			&cfg.GroupName,
			&cfg.ProviderName,
			&cfg.IsActive,
			&cfg.SecretID,
			&cfg.KeyPrefix,
			&cfg.BaseURL,
			&cfg.ConfigJSON,
			&cfg.CreatedAt,
			&cfg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (r *ToolProviderConfigsRepository) UpsertConfig(
	ctx context.Context,
	projectID uuid.UUID,
	scope string, // "project" | "platform"
	groupName string,
	providerName string,
	secretID *uuid.UUID,
	keyPrefix *string,
	baseURL *string,
	configJSON []byte,
) (ToolProviderConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if scope != "project" && scope != "platform" {
		return ToolProviderConfig{}, fmt.Errorf("scope must be project or platform")
	}
	if scope == "project" && projectID == uuid.Nil {
		return ToolProviderConfig{}, fmt.Errorf("project_id must not be empty for project scope")
	}

	group := strings.TrimSpace(groupName)
	provider := strings.TrimSpace(providerName)
	if group == "" {
		return ToolProviderConfig{}, fmt.Errorf("group_name must not be empty")
	}
	if provider == "" {
		return ToolProviderConfig{}, fmt.Errorf("provider_name must not be empty")
	}

	if configJSON != nil && len(configJSON) == 0 {
		configJSON = []byte("{}")
	}

	var projectIDParam any
	if scope == "platform" {
		projectIDParam = nil
	} else {
		projectIDParam = projectID
	}

	var cfg ToolProviderConfig
	var row pgx.Row
	if scope == "platform" {
		row = r.db.QueryRow(ctx, `
			INSERT INTO tool_provider_configs (project_id, scope, group_name, provider_name, secret_id, key_prefix, base_url, config_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, '{}'::jsonb))
			ON CONFLICT (provider_name) WHERE scope = 'platform'
			DO UPDATE SET
				group_name  = EXCLUDED.group_name,
				secret_id   = COALESCE(EXCLUDED.secret_id, tool_provider_configs.secret_id),
				key_prefix  = COALESCE(EXCLUDED.key_prefix, tool_provider_configs.key_prefix),
				base_url    = COALESCE(EXCLUDED.base_url, tool_provider_configs.base_url),
				config_json = CASE WHEN $8 IS NULL THEN tool_provider_configs.config_json ELSE EXCLUDED.config_json END,
				updated_at  = now()
			RETURNING id, org_id, scope, project_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
		`, projectIDParam, scope, group, provider, secretID, keyPrefix, baseURL, configJSON)
	} else {
		row = r.db.QueryRow(ctx, `
			INSERT INTO tool_provider_configs (project_id, scope, group_name, provider_name, secret_id, key_prefix, base_url, config_json)
			VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, '{}'::jsonb))
			ON CONFLICT (project_id, provider_name) WHERE project_id IS NOT NULL
			DO UPDATE SET
				group_name  = EXCLUDED.group_name,
				secret_id   = COALESCE(EXCLUDED.secret_id, tool_provider_configs.secret_id),
				key_prefix  = COALESCE(EXCLUDED.key_prefix, tool_provider_configs.key_prefix),
				base_url    = COALESCE(EXCLUDED.base_url, tool_provider_configs.base_url),
				config_json = CASE WHEN $8 IS NULL THEN tool_provider_configs.config_json ELSE EXCLUDED.config_json END,
				updated_at  = now()
			RETURNING id, org_id, scope, project_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
		`, projectIDParam, scope, group, provider, secretID, keyPrefix, baseURL, configJSON)
	}

	err := row.Scan(
		&cfg.ID,
		&cfg.OrgID,
		&cfg.Scope,
		&cfg.ProjectID,
		&cfg.GroupName,
		&cfg.ProviderName,
		&cfg.IsActive,
		&cfg.SecretID,
		&cfg.KeyPrefix,
		&cfg.BaseURL,
		&cfg.ConfigJSON,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return ToolProviderConfig{}, err
	}
	return cfg, nil
}

// Activate 事务内调用：同组全关 + 指定 provider 打开。
func (r *ToolProviderConfigsRepository) Activate(ctx context.Context, projectID uuid.UUID, scope string, groupName string, providerName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if scope != "project" && scope != "platform" {
		return fmt.Errorf("scope must be project or platform")
	}
	if scope == "project" && projectID == uuid.Nil {
		return fmt.Errorf("project_id must not be empty for project scope")
	}
	group := strings.TrimSpace(groupName)
	provider := strings.TrimSpace(providerName)
	if group == "" || provider == "" {
		return fmt.Errorf("group_name and provider_name must not be empty")
	}

	if scope == "platform" {
		if _, err := r.db.Exec(ctx, `
			UPDATE tool_provider_configs
			SET is_active = FALSE, updated_at = now()
			WHERE scope = 'platform' AND group_name = $1 AND is_active = TRUE
		`, group); err != nil {
			return err
		}

		_, err := r.db.Exec(ctx, `
			INSERT INTO tool_provider_configs (project_id, scope, group_name, provider_name, is_active)
			VALUES (NULL, 'platform', $1, $2, TRUE)
			ON CONFLICT (provider_name) WHERE scope = 'platform'
			DO UPDATE SET
				group_name = EXCLUDED.group_name,
				is_active  = TRUE,
				updated_at = now()
		`, group, provider)
		return err
	}

	if _, err := r.db.Exec(ctx, `
		UPDATE tool_provider_configs
		SET is_active = FALSE, updated_at = now()
		WHERE project_id = $1 AND scope = 'project' AND group_name = $2 AND is_active = TRUE
	`, projectID, group); err != nil {
		return err
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO tool_provider_configs (project_id, scope, group_name, provider_name, is_active)
		VALUES ($1, 'project', $2, $3, TRUE)
		ON CONFLICT (project_id, provider_name) WHERE project_id IS NOT NULL
		DO UPDATE SET
			group_name = EXCLUDED.group_name,
			is_active  = TRUE,
			updated_at = now()
	`, projectID, group, provider)
	return err
}

func (r *ToolProviderConfigsRepository) Deactivate(ctx context.Context, projectID uuid.UUID, scope string, groupName string, providerName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if scope != "project" && scope != "platform" {
		return fmt.Errorf("scope must be project or platform")
	}
	if scope == "project" && projectID == uuid.Nil {
		return fmt.Errorf("project_id must not be empty for project scope")
	}
	group := strings.TrimSpace(groupName)
	provider := strings.TrimSpace(providerName)
	if group == "" || provider == "" {
		return fmt.Errorf("group_name and provider_name must not be empty")
	}

	if scope == "platform" {
		_, err := r.db.Exec(ctx, `
			UPDATE tool_provider_configs
			SET is_active = FALSE, updated_at = now()
			WHERE scope = 'platform' AND group_name = $1 AND provider_name = $2
		`, group, provider)
		return err
	}

	_, err := r.db.Exec(ctx, `
		UPDATE tool_provider_configs
		SET is_active = FALSE, updated_at = now()
		WHERE project_id = $1 AND scope = 'project' AND group_name = $2 AND provider_name = $3
	`, projectID, group, provider)
	return err
}

func (r *ToolProviderConfigsRepository) ClearCredential(ctx context.Context, projectID uuid.UUID, scope string, providerName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if scope != "project" && scope != "platform" {
		return fmt.Errorf("scope must be project or platform")
	}
	if scope == "project" && projectID == uuid.Nil {
		return fmt.Errorf("project_id must not be empty for project scope")
	}
	provider := strings.TrimSpace(providerName)
	if provider == "" {
		return fmt.Errorf("provider_name must not be empty")
	}

	if scope == "platform" {
		_, err := r.db.Exec(ctx, `
			UPDATE tool_provider_configs
			SET is_active = FALSE,
			    secret_id = NULL,
			    key_prefix = NULL,
			    base_url = NULL,
			    config_json = '{}'::jsonb,
			    updated_at = now()
			WHERE scope = 'platform' AND provider_name = $1
		`, provider)
		return err
	}

	_, err := r.db.Exec(ctx, `
		UPDATE tool_provider_configs
		SET is_active = FALSE,
		    secret_id = NULL,
		    key_prefix = NULL,
		    base_url = NULL,
		    config_json = '{}'::jsonb,
		    updated_at = now()
		WHERE project_id = $1 AND scope = 'project' AND provider_name = $2
	`, projectID, provider)
	return err
}
