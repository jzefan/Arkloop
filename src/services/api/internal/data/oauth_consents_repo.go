package data

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OAuthConsent 记录某用户对某 client 已授权的 scope 并集。
// 后续同 client 的授权请求只要请求 scope ⊆ 这里的 scopes，就静默通过；
// 一旦请求了更多 scope，要重新弹同意页（并 union 入库）。
type OAuthConsent struct {
	UserID    uuid.UUID
	ClientID  string
	Scopes    []string
	GrantedAt time.Time
	RevokedAt *time.Time
}

type OAuthConsentRepository struct {
	db Querier
}

func NewOAuthConsentRepository(db Querier) (*OAuthConsentRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &OAuthConsentRepository{db: db}, nil
}

// Get 返回有效的 consent。已撤销或不存在返回 nil, nil。
func (r *OAuthConsentRepository) Get(
	ctx context.Context,
	userID uuid.UUID,
	clientID string,
) (*OAuthConsent, error) {
	var c OAuthConsent
	err := r.db.QueryRow(
		ctx,
		`SELECT user_id, client_id, scopes, granted_at, revoked_at
		 FROM oauth_consents
		 WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`,
		userID, clientID,
	).Scan(&c.UserID, &c.ClientID, &c.Scopes, &c.GrantedAt, &c.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// Grant 写入或合并 consent。新请求的 scopes 与已有 scopes 取并集后落库。
// 若记录已被撤销（revoked_at != NULL），按"重新授权"语义清空 revoked_at 并覆盖 scopes。
func (r *OAuthConsentRepository) Grant(
	ctx context.Context,
	userID uuid.UUID,
	clientID string,
	scopes []string,
) error {
	if userID == uuid.Nil {
		return errors.New("user_id must not be nil")
	}
	if clientID == "" {
		return errors.New("client_id must not be empty")
	}
	if scopes == nil {
		scopes = []string{}
	}
	// ON CONFLICT 合并：union 已有 + 新申请的 scopes，去重保序。
	_, err := r.db.Exec(
		ctx,
		`INSERT INTO oauth_consents (user_id, client_id, scopes, granted_at, revoked_at)
		 VALUES ($1, $2, $3, now(), NULL)
		 ON CONFLICT (user_id, client_id) DO UPDATE
		 SET scopes = (
		     SELECT ARRAY(SELECT DISTINCT unnest(oauth_consents.scopes || EXCLUDED.scopes))
		 ),
		 granted_at = now(),
		 revoked_at = NULL`,
		userID, clientID, scopes,
	)
	return err
}

// Revoke 撤销 consent。同时调用方应一并撤销相关 refresh token（见 oauth_refresh_tokens_repo）。
func (r *OAuthConsentRepository) Revoke(
	ctx context.Context,
	userID uuid.UUID,
	clientID string,
) error {
	_, err := r.db.Exec(
		ctx,
		`UPDATE oauth_consents
		 SET revoked_at = now()
		 WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`,
		userID, clientID,
	)
	return err
}
