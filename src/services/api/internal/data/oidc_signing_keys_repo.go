package data

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// OIDCSigningKey 是 OIDC token 的签名密钥对。private_key_encrypted
// 是经 sharedencryption.KeyRing 加密后的字符串，配套 KeyVersion 用于解密。
type OIDCSigningKey struct {
	KID                        string
	Algorithm                  string
	PublicKeyPEM               string
	PrivateKeyEncrypted        string
	PrivateKeyEncryptionKeyVer int
	Status                     string // active | retired | compromised
	CreatedAt                  time.Time
	ActivatedAt                *time.Time
	RetiredAt                  *time.Time
}

type OIDCSigningKeyRepository struct {
	db Querier
}

func NewOIDCSigningKeyRepository(db Querier) (*OIDCSigningKeyRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &OIDCSigningKeyRepository{db: db}, nil
}

// Insert 写入新生成的密钥对。activate=true 时一并把 activated_at 置为 now()。
func (r *OIDCSigningKeyRepository) Insert(
	ctx context.Context,
	kid, algorithm, publicKeyPEM, privateKeyEncrypted string,
	keyVersion int,
	activate bool,
) (OIDCSigningKey, error) {
	if kid == "" {
		return OIDCSigningKey{}, errors.New("kid must not be empty")
	}
	if algorithm == "" {
		algorithm = "RS256"
	}
	activatedAt := "NULL"
	if activate {
		activatedAt = "now()"
	}

	var k OIDCSigningKey
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO oidc_signing_keys
			(kid, algorithm, public_key_pem, private_key_encrypted, private_key_encryption_keyver, status, activated_at)
		 VALUES ($1, $2, $3, $4, $5, 'active', `+activatedAt+`)
		 RETURNING kid, algorithm, public_key_pem, private_key_encrypted, private_key_encryption_keyver,
		           status, created_at, activated_at, retired_at`,
		kid, algorithm, publicKeyPEM, privateKeyEncrypted, keyVersion,
	).Scan(&k.KID, &k.Algorithm, &k.PublicKeyPEM, &k.PrivateKeyEncrypted, &k.PrivateKeyEncryptionKeyVer,
		&k.Status, &k.CreatedAt, &k.ActivatedAt, &k.RetiredAt)
	if err != nil {
		return OIDCSigningKey{}, err
	}
	return k, nil
}

// GetActive 返回当前用于签发新 token 的 key。多个 active 时返回最新（不应发生，约束保护）。
func (r *OIDCSigningKeyRepository) GetActive(ctx context.Context) (*OIDCSigningKey, error) {
	var k OIDCSigningKey
	err := r.db.QueryRow(
		ctx,
		`SELECT kid, algorithm, public_key_pem, private_key_encrypted, private_key_encryption_keyver,
		        status, created_at, activated_at, retired_at
		 FROM oidc_signing_keys
		 WHERE status = 'active'
		 ORDER BY created_at DESC
		 LIMIT 1`,
	).Scan(&k.KID, &k.Algorithm, &k.PublicKeyPEM, &k.PrivateKeyEncrypted, &k.PrivateKeyEncryptionKeyVer,
		&k.Status, &k.CreatedAt, &k.ActivatedAt, &k.RetiredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &k, nil
}

// GetByKID 按 kid 取一条记录，不论 status。用于 token 验签时按 header.kid 查找。
func (r *OIDCSigningKeyRepository) GetByKID(ctx context.Context, kid string) (*OIDCSigningKey, error) {
	var k OIDCSigningKey
	err := r.db.QueryRow(
		ctx,
		`SELECT kid, algorithm, public_key_pem, private_key_encrypted, private_key_encryption_keyver,
		        status, created_at, activated_at, retired_at
		 FROM oidc_signing_keys
		 WHERE kid = $1`,
		kid,
	).Scan(&k.KID, &k.Algorithm, &k.PublicKeyPEM, &k.PrivateKeyEncrypted, &k.PrivateKeyEncryptionKeyVer,
		&k.Status, &k.CreatedAt, &k.ActivatedAt, &k.RetiredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &k, nil
}

// ListPublishable 返回 JWKS 端点应公开的所有 key (active + retired)。
// compromised 的密钥被立即下架，不再返回。
func (r *OIDCSigningKeyRepository) ListPublishable(ctx context.Context) ([]OIDCSigningKey, error) {
	rows, err := r.db.Query(
		ctx,
		`SELECT kid, algorithm, public_key_pem, private_key_encrypted, private_key_encryption_keyver,
		        status, created_at, activated_at, retired_at
		 FROM oidc_signing_keys
		 WHERE status IN ('active', 'retired')
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []OIDCSigningKey
	for rows.Next() {
		var k OIDCSigningKey
		if err := rows.Scan(&k.KID, &k.Algorithm, &k.PublicKeyPEM, &k.PrivateKeyEncrypted, &k.PrivateKeyEncryptionKeyVer,
			&k.Status, &k.CreatedAt, &k.ActivatedAt, &k.RetiredAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// UpdateStatus 修改 key 状态。retire 时一并写 retired_at；compromise 时立即下架。
func (r *OIDCSigningKeyRepository) UpdateStatus(ctx context.Context, kid, newStatus string) error {
	switch newStatus {
	case "active", "retired", "compromised":
	default:
		return errors.New("invalid status: " + newStatus)
	}

	var sql string
	switch newStatus {
	case "retired":
		sql = `UPDATE oidc_signing_keys SET status = 'retired', retired_at = COALESCE(retired_at, now()) WHERE kid = $1`
	case "compromised":
		sql = `UPDATE oidc_signing_keys SET status = 'compromised', retired_at = COALESCE(retired_at, now()) WHERE kid = $1`
	default:
		sql = `UPDATE oidc_signing_keys SET status = $2 WHERE kid = $1`
	}

	if newStatus == "active" {
		_, err := r.db.Exec(ctx, sql, kid, newStatus)
		return err
	}
	_, err := r.db.Exec(ctx, sql, kid)
	return err
}
