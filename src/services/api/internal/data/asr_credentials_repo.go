package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type AsrCredentialNameConflictError struct{ Name string }

func (e AsrCredentialNameConflictError) Error() string {
	return fmt.Sprintf("asr credential %q already exists", e.Name)
}

type AsrCredential struct {
	ID        uuid.UUID
	OrgID     *uuid.UUID // nil = platform scope
	Scope     string     // "org" | "platform"
	Provider  string
	Name      string
	SecretID  *uuid.UUID
	KeyPrefix *string
	BaseURL   *string
	Model     string
	IsDefault bool
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AsrCredentialsRepository struct {
	db Querier
}

func NewAsrCredentialsRepository(db Querier) (*AsrCredentialsRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &AsrCredentialsRepository{db: db}, nil
}

func (r *AsrCredentialsRepository) WithTx(tx database.Tx) *AsrCredentialsRepository {
	return &AsrCredentialsRepository{db: tx}
}

func (r *AsrCredentialsRepository) Create(
	ctx context.Context,
	id uuid.UUID,
	orgID uuid.UUID, // uuid.Nil for platform scope
	scope string,    // "org" | "platform"
	provider string,
	name string,
	secretID *uuid.UUID,
	keyPrefix *string,
	baseURL *string,
	model string,
	isDefault bool,
) (AsrCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if id == uuid.Nil {
		return AsrCredential{}, fmt.Errorf("id must not be nil")
	}
	if scope != "org" && scope != "platform" {
		return AsrCredential{}, fmt.Errorf("scope must be org or platform")
	}
	if scope == "org" && orgID == uuid.Nil {
		return AsrCredential{}, fmt.Errorf("org_id must not be nil for org scope")
	}
	if strings.TrimSpace(provider) == "" {
		return AsrCredential{}, fmt.Errorf("provider must not be empty")
	}
	if strings.TrimSpace(name) == "" {
		return AsrCredential{}, fmt.Errorf("name must not be empty")
	}
	if strings.TrimSpace(model) == "" {
		return AsrCredential{}, fmt.Errorf("model must not be empty")
	}

	var orgIDParam any
	if scope == "platform" {
		orgIDParam = nil
	} else {
		orgIDParam = orgID
	}

	var c AsrCredential
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO asr_credentials
		    (id, org_id, scope, provider, name, secret_id, key_prefix, base_url, model, is_default)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id, org_id, scope, provider, name, secret_id, key_prefix,
		           base_url, model, is_default, revoked_at, created_at, updated_at`,
		id, orgIDParam, scope, provider, name, secretID, keyPrefix, baseURL, model, isDefault,
	).Scan(
		&c.ID, &c.OrgID, &c.Scope, &c.Provider, &c.Name, &c.SecretID, &c.KeyPrefix,
		&c.BaseURL, &c.Model, &c.IsDefault, &c.RevokedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return AsrCredential{}, AsrCredentialNameConflictError{Name: name}
		}
		return AsrCredential{}, err
	}
	return c, nil
}

func (r *AsrCredentialsRepository) GetByID(ctx context.Context, orgID, id uuid.UUID) (*AsrCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var c AsrCredential
	err := r.db.QueryRow(
		ctx,
		`SELECT id, org_id, scope, provider, name, secret_id, key_prefix,
		        base_url, model, is_default, revoked_at, created_at, updated_at
		 FROM asr_credentials
		 WHERE id = $1 AND (org_id = $2 OR scope = 'platform')`,
		id, orgID,
	).Scan(
		&c.ID, &c.OrgID, &c.Scope, &c.Provider, &c.Name, &c.SecretID, &c.KeyPrefix,
		&c.BaseURL, &c.Model, &c.IsDefault, &c.RevokedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// GetDefault 返回 org 级 default，找不到时 fallback 到 platform 级 default。
func (r *AsrCredentialsRepository) GetDefault(ctx context.Context, orgID uuid.UUID) (*AsrCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var c AsrCredential
	err := r.db.QueryRow(
		ctx,
		`(SELECT id, org_id, scope, provider, name, secret_id, key_prefix,
		         base_url, model, is_default, revoked_at, created_at, updated_at
		  FROM asr_credentials
		  WHERE org_id = $1 AND scope = 'org' AND is_default = true AND revoked_at IS NULL
		  LIMIT 1)
		 UNION ALL
		 (SELECT id, org_id, scope, provider, name, secret_id, key_prefix,
		         base_url, model, is_default, revoked_at, created_at, updated_at
		  FROM asr_credentials
		  WHERE scope = 'platform' AND is_default = true AND revoked_at IS NULL
		  LIMIT 1)
		 LIMIT 1`,
		orgID,
	).Scan(
		&c.ID, &c.OrgID, &c.Scope, &c.Provider, &c.Name, &c.SecretID, &c.KeyPrefix,
		&c.BaseURL, &c.Model, &c.IsDefault, &c.RevokedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *AsrCredentialsRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]AsrCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := r.db.Query(
		ctx,
		`SELECT id, org_id, scope, provider, name, secret_id, key_prefix,
		        base_url, model, is_default, revoked_at, created_at, updated_at
		 FROM asr_credentials
		 WHERE ((org_id = $1 AND scope = 'org') OR scope = 'platform') AND revoked_at IS NULL
		 ORDER BY scope ASC, created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	creds := []AsrCredential{}
	for rows.Next() {
		var c AsrCredential
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.Scope, &c.Provider, &c.Name, &c.SecretID, &c.KeyPrefix,
			&c.BaseURL, &c.Model, &c.IsDefault, &c.RevokedAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// SetDefault 原子地将指定 org 内的凭证设为 default，同时清除同 org 其他凭证的 default 标记。
func (r *AsrCredentialsRepository) SetDefault(ctx context.Context, orgID, id uuid.UUID) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := r.db.Exec(
		ctx,
		`UPDATE asr_credentials
		 SET is_default = (id = $2), updated_at = now()
		 WHERE org_id = $1 AND scope = 'org' AND revoked_at IS NULL`,
		orgID, id,
	)
	return err
}

// SetDefaultPlatform 原子地将 platform scope 凭证设为 default。
func (r *AsrCredentialsRepository) SetDefaultPlatform(ctx context.Context, id uuid.UUID) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := r.db.Exec(
		ctx,
		`UPDATE asr_credentials
		 SET is_default = (id = $1), updated_at = now()
		 WHERE scope = 'platform' AND revoked_at IS NULL`,
		id,
	)
	return err
}

func (r *AsrCredentialsRepository) Delete(ctx context.Context, orgID, id uuid.UUID, isPlatformAdmin bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	if isPlatformAdmin {
		_, err = r.db.Exec(
			ctx,
			`DELETE FROM asr_credentials WHERE id = $1 AND (org_id = $2 OR scope = 'platform')`,
			id, orgID,
		)
	} else {
		_, err = r.db.Exec(
			ctx,
			`DELETE FROM asr_credentials WHERE id = $1 AND org_id = $2 AND scope = 'org'`,
			id, orgID,
		)
	}
	return err
}
