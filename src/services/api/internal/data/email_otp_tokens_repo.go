package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type EmailOTPToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type EmailOTPTokenRepository struct {
	db Querier
}

func NewEmailOTPTokenRepository(db Querier) (*EmailOTPTokenRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &EmailOTPTokenRepository{db: db}, nil
}

func (r *EmailOTPTokenRepository) Create(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash string,
	expiresAt time.Time,
) (EmailOTPToken, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if userID == uuid.Nil {
		return EmailOTPToken{}, fmt.Errorf("user_id must not be empty")
	}
	if tokenHash == "" {
		return EmailOTPToken{}, fmt.Errorf("token_hash must not be empty")
	}

	var t EmailOTPToken
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO email_otp_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, user_id, token_hash, expires_at, used_at, created_at`,
		userID, tokenHash, expiresAt.UTC(),
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		return EmailOTPToken{}, err
	}
	return t, nil
}

// Consume 原子地标记 token 为已使用并返回对应的 user_id。
// 只有未使用且未过期的 token 才会被消耗。
func (r *EmailOTPTokenRepository) Consume(
	ctx context.Context,
	tokenHash string,
) (uuid.UUID, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var userID uuid.UUID
	err := r.db.QueryRow(
		ctx,
		`UPDATE email_otp_tokens
		 SET used_at = now()
		 WHERE token_hash = $1
		   AND used_at IS NULL
		   AND expires_at > now()
		 RETURNING user_id`,
		tokenHash,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, fmt.Errorf("email_otp_tokens.Consume: %w", err)
	}
	return userID, true, nil
}
