package data

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type RefreshToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

type RefreshTokenRepository struct {
	db Querier
}

func NewRefreshTokenRepository(db Querier) (*RefreshTokenRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &RefreshTokenRepository{db: db}, nil
}

func (r *RefreshTokenRepository) Create(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (RefreshToken, error) {
	if userID == uuid.Nil {
		return RefreshToken{}, errors.New("user_id must not be nil")
	}
	if tokenHash == "" {
		return RefreshToken{}, errors.New("token_hash must not be empty")
	}

	var t RefreshToken
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at, last_used_at`,
		userID, tokenHash, expiresAt,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.LastUsedAt)
	if err != nil {
		return RefreshToken{}, err
	}
	return t, nil
}

// GetByHash 只返回未吊销且未过期的 token。
func (r *RefreshTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var t RefreshToken
	err := r.db.QueryRow(
		ctx,
		`SELECT id, user_id, token_hash, expires_at, revoked_at, created_at, last_used_at
		 FROM refresh_tokens
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > now()`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt, &t.LastUsedAt)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

// RevokeByHash 吊销指定 hash 的 token。返回 false 表示 token 不存在或已失效（重放检测）。
func (r *RefreshTokenRepository) RevokeByHash(ctx context.Context, tokenHash string) (bool, error) {
	tag, err := r.db.Exec(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = now()
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > now()`,
		tokenHash,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ConsumeByHash 原子性地吊销 token 并返回其关联的 user_id，用于 Refresh Token 轮换。
// 若 token 不存在、已过期或已吊销则返回 uuid.Nil, false, nil（重放检测）。
func (r *RefreshTokenRepository) ConsumeByHash(ctx context.Context, tokenHash string) (uuid.UUID, bool, error) {
	var userID uuid.UUID
	err := r.db.QueryRow(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = now(), last_used_at = now()
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > now()
		 RETURNING user_id`,
		tokenHash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return userID, true, nil
}

// RevokeAllForUser 吊销指定用户的所有有效 Refresh Token。
func (r *RefreshTokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE refresh_tokens
		 SET revoked_at = now()
		 WHERE user_id = $1
		   AND revoked_at IS NULL`,
		userID,
	)
	return err
}
