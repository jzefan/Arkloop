package data

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OAuthAuthorizationCode 是 Authorization Code Flow 的一次性 code 凭证。
// 数据库中只保存 SHA256(plain_code) 作为 PK，明文 code 仅在颁发瞬间存在于内存。
type OAuthAuthorizationCode struct {
	CodeHash            string
	ClientID            string
	UserID              uuid.UUID
	RedirectURI         string
	Scopes              []string
	CodeChallenge       string
	CodeChallengeMethod string
	Nonce               *string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
	CreatedAt           time.Time
}

type OAuthAuthorizationCodeRepository struct {
	db Querier
}

func NewOAuthAuthorizationCodeRepository(db Querier) (*OAuthAuthorizationCodeRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &OAuthAuthorizationCodeRepository{db: db}, nil
}

// Create 插入一条新 code。调用方负责生成 plain code 并算 SHA256。
func (r *OAuthAuthorizationCodeRepository) Create(
	ctx context.Context,
	codeHash, clientID string,
	userID uuid.UUID,
	redirectURI string,
	scopes []string,
	codeChallenge, codeChallengeMethod string,
	nonce *string,
	expiresAt time.Time,
) error {
	if codeHash == "" {
		return errors.New("code_hash must not be empty")
	}
	if userID == uuid.Nil {
		return errors.New("user_id must not be nil")
	}
	if codeChallenge == "" {
		return errors.New("code_challenge must not be empty (PKCE required)")
	}
	if scopes == nil {
		scopes = []string{}
	}

	_, err := r.db.Exec(
		ctx,
		`INSERT INTO oauth_authorization_codes
			(code_hash, client_id, user_id, redirect_uri, scopes,
			 code_challenge, code_challenge_method, nonce, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		codeHash, clientID, userID, redirectURI, scopes,
		codeChallenge, codeChallengeMethod, nonce, expiresAt,
	)
	return err
}

// Consume 原子性地标记 code 为已使用并返回其内容。重放 / 过期 / 已用返回 nil, nil。
//
// 一旦 RETURNING 命中，code 即被锁定为 consumed，后续任何 Consume 都会落空：
// 这是单次使用约束的实现，保护了 Authorization Code 不被重放。
func (r *OAuthAuthorizationCodeRepository) Consume(
	ctx context.Context,
	codeHash string,
) (*OAuthAuthorizationCode, error) {
	var c OAuthAuthorizationCode
	err := r.db.QueryRow(
		ctx,
		`UPDATE oauth_authorization_codes
		 SET consumed_at = now()
		 WHERE code_hash = $1
		   AND consumed_at IS NULL
		   AND expires_at > now()
		 RETURNING code_hash, client_id, user_id, redirect_uri, scopes,
		           code_challenge, code_challenge_method, nonce, expires_at, consumed_at, created_at`,
		codeHash,
	).Scan(&c.CodeHash, &c.ClientID, &c.UserID, &c.RedirectURI, &c.Scopes,
		&c.CodeChallenge, &c.CodeChallengeMethod, &c.Nonce, &c.ExpiresAt, &c.ConsumedAt, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// DeleteExpired 清理过期或已使用超过 1 天的 code。后台维护任务定期调用。
func (r *OAuthAuthorizationCodeRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(
		ctx,
		`DELETE FROM oauth_authorization_codes
		 WHERE expires_at < now() - interval '1 hour'
		    OR (consumed_at IS NOT NULL AND consumed_at < now() - interval '1 day')`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
