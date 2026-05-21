package data

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OAuthClient 描述一个已注册的 OAuth/OIDC 客户端应用（如 exam-web）。
// client_secret 永远不暴露到内存外，仅以 bcrypt hash 形式保存。
type OAuthClient struct {
	ID                uuid.UUID
	ClientID          string
	ClientSecretHash  string
	ClientType        string // "confidential" | "public"
	Name              string
	RedirectURIs      []string
	AllowedScopes     []string
	RequirePKCE       bool
	CreatedAt         time.Time
	DeletedAt         *time.Time
}

type OAuthClientRepository struct {
	db Querier
}

func NewOAuthClientRepository(db Querier) (*OAuthClientRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &OAuthClientRepository{db: db}, nil
}

// Create 注册一个新的 client。client_secret_hash 由调用方提前 bcrypt 计算。
func (r *OAuthClientRepository) Create(
	ctx context.Context,
	clientID, clientSecretHash, clientType, name string,
	redirectURIs, allowedScopes []string,
	requirePKCE bool,
) (OAuthClient, error) {
	if clientID == "" {
		return OAuthClient{}, errors.New("client_id must not be empty")
	}
	if clientSecretHash == "" {
		return OAuthClient{}, errors.New("client_secret_hash must not be empty")
	}
	if clientType != "confidential" && clientType != "public" {
		return OAuthClient{}, errors.New("client_type must be confidential or public")
	}
	if len(redirectURIs) == 0 {
		return OAuthClient{}, errors.New("redirect_uris must not be empty")
	}
	if allowedScopes == nil {
		allowedScopes = []string{}
	}

	var c OAuthClient
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO oauth_clients (client_id, client_secret_hash, client_type, name, redirect_uris, allowed_scopes, require_pkce)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, client_id, client_secret_hash, client_type, name, redirect_uris, allowed_scopes, require_pkce, created_at, deleted_at`,
		clientID, clientSecretHash, clientType, name, redirectURIs, allowedScopes, requirePKCE,
	).Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.ClientType, &c.Name,
		&c.RedirectURIs, &c.AllowedScopes, &c.RequirePKCE, &c.CreatedAt, &c.DeletedAt)
	if err != nil {
		return OAuthClient{}, err
	}
	return c, nil
}

// GetByClientID 返回未软删除的 client。不存在返回 nil, nil。
func (r *OAuthClientRepository) GetByClientID(ctx context.Context, clientID string) (*OAuthClient, error) {
	var c OAuthClient
	err := r.db.QueryRow(
		ctx,
		`SELECT id, client_id, client_secret_hash, client_type, name, redirect_uris, allowed_scopes, require_pkce, created_at, deleted_at
		 FROM oauth_clients
		 WHERE client_id = $1 AND deleted_at IS NULL`,
		clientID,
	).Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.ClientType, &c.Name,
		&c.RedirectURIs, &c.AllowedScopes, &c.RequirePKCE, &c.CreatedAt, &c.DeletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// List 返回所有未软删除的 client。
func (r *OAuthClientRepository) List(ctx context.Context) ([]OAuthClient, error) {
	rows, err := r.db.Query(
		ctx,
		`SELECT id, client_id, client_secret_hash, client_type, name, redirect_uris, allowed_scopes, require_pkce, created_at, deleted_at
		 FROM oauth_clients
		 WHERE deleted_at IS NULL
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []OAuthClient
	for rows.Next() {
		var c OAuthClient
		if err := rows.Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.ClientType, &c.Name,
			&c.RedirectURIs, &c.AllowedScopes, &c.RequirePKCE, &c.CreatedAt, &c.DeletedAt); err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

// SoftDelete 标记 client 为已删除。后续 GetByClientID 将返回 nil。
func (r *OAuthClientRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE oauth_clients SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	return err
}
