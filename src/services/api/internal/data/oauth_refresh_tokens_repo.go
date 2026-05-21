package data

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OAuthRefreshToken 是 OIDC 长期凭证。区别于已存在的 refresh_tokens 表
// （后者服务于 ArkLoop 一方的浏览器 session），本表按 (user_id, client_id) 颁发，
// 并通过 rotated_to 串成链以支持 rotation-on-use + replay detection。
type OAuthRefreshToken struct {
	TokenHash string
	ClientID  string
	UserID    uuid.UUID
	Scopes    []string
	IssuedAt  time.Time
	ExpiresAt time.Time
	RotatedTo *string
	RevokedAt *time.Time
}

type OAuthRefreshTokenRepository struct {
	db Querier
}

func NewOAuthRefreshTokenRepository(db Querier) (*OAuthRefreshTokenRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &OAuthRefreshTokenRepository{db: db}, nil
}

// Create 插入新颁发的 token。
func (r *OAuthRefreshTokenRepository) Create(
	ctx context.Context,
	tokenHash, clientID string,
	userID uuid.UUID,
	scopes []string,
	expiresAt time.Time,
) error {
	if tokenHash == "" {
		return errors.New("token_hash must not be empty")
	}
	if userID == uuid.Nil {
		return errors.New("user_id must not be nil")
	}
	if scopes == nil {
		scopes = []string{}
	}
	_, err := r.db.Exec(
		ctx,
		`INSERT INTO oauth_refresh_tokens (token_hash, client_id, user_id, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		tokenHash, clientID, userID, scopes, expiresAt,
	)
	return err
}

// GetByHash 查询 token 的最新状态（含 revoked 和 rotated_to），供调用方判定。
// 不存在返回 nil, nil。
func (r *OAuthRefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*OAuthRefreshToken, error) {
	var t OAuthRefreshToken
	err := r.db.QueryRow(
		ctx,
		`SELECT token_hash, client_id, user_id, scopes, issued_at, expires_at, rotated_to, revoked_at
		 FROM oauth_refresh_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.TokenHash, &t.ClientID, &t.UserID, &t.Scopes,
		&t.IssuedAt, &t.ExpiresAt, &t.RotatedTo, &t.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// Rotate 在单事务内 revoke 老 token 并写入新 token + 建立 rotated_to 链。
// 老 token 必须当前未过期且未撤销，否则 ok=false（重放尝试）。
// 调用方应在 ok=false 时调用 RevokeChain(oldHash) 进入"被盗用"流程。
func (r *OAuthRefreshTokenRepository) Rotate(
	ctx context.Context,
	tx DB,
	oldHash, newHash string,
	scopes []string,
	expiresAt time.Time,
) (ok bool, err error) {
	if tx == nil {
		return false, errors.New("tx must not be nil")
	}
	txn, err := tx.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = txn.Rollback(ctx)
		}
	}()

	var old OAuthRefreshToken
	err = txn.QueryRow(
		ctx,
		`UPDATE oauth_refresh_tokens
		 SET revoked_at = now(), rotated_to = $2
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > now()
		 RETURNING token_hash, client_id, user_id, scopes, issued_at, expires_at, rotated_to, revoked_at`,
		oldHash, newHash,
	).Scan(&old.TokenHash, &old.ClientID, &old.UserID, &old.Scopes,
		&old.IssuedAt, &old.ExpiresAt, &old.RotatedTo, &old.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = txn.Rollback(ctx)
			return false, nil
		}
		return false, err
	}

	if scopes == nil {
		scopes = []string{}
	}
	if _, err = txn.Exec(
		ctx,
		`INSERT INTO oauth_refresh_tokens (token_hash, client_id, user_id, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		newHash, old.ClientID, old.UserID, scopes, expiresAt,
	); err != nil {
		return false, err
	}

	if err = txn.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// RevokeChain 沿着 rotated_to 链向后递归 revoke 所有未撤销节点。
// 用于"refresh token 重放检测"：发现 revoked 节点被重用时，假定 token 已被盗，
// 一次性撤销整条 rotation 链，强制用户重新登录。
func (r *OAuthRefreshTokenRepository) RevokeChain(ctx context.Context, startHash string) error {
	_, err := r.db.Exec(
		ctx,
		`WITH RECURSIVE chain AS (
		    SELECT token_hash, rotated_to FROM oauth_refresh_tokens WHERE token_hash = $1
		  UNION ALL
		    SELECT t.token_hash, t.rotated_to
		    FROM oauth_refresh_tokens t JOIN chain c ON t.token_hash = c.rotated_to
		 )
		 UPDATE oauth_refresh_tokens
		 SET revoked_at = COALESCE(revoked_at, now())
		 WHERE token_hash IN (SELECT token_hash FROM chain)`,
		startHash,
	)
	return err
}

// RevokeAllForUserClient 撤销该 (user, client) 下所有未撤销的 token。
// 用户主动撤销 consent 时调用。
func (r *OAuthRefreshTokenRepository) RevokeAllForUserClient(
	ctx context.Context,
	userID uuid.UUID,
	clientID string,
) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE oauth_refresh_tokens
		 SET revoked_at = now()
		 WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`,
		userID, clientID,
	)
	return err
}

// DeleteExpired 物理删除已过期 30 天以上的记录（已 revoked 链可保留更长用于审计）。
func (r *OAuthRefreshTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(
		ctx,
		`DELETE FROM oauth_refresh_tokens
		 WHERE expires_at < now() - interval '30 days'`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
