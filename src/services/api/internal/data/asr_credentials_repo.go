package data

import (
"context"
"errors"
"fmt"
"strings"
"time"

"github.com/google/uuid"
"github.com/jackc/pgx/v5"
)

type AsrCredentialNameConflictError struct{ Name string }

func (e AsrCredentialNameConflictError) Error() string {
return fmt.Sprintf("asr credential %q already exists", e.Name)
}

type AsrCredential struct {
ID          uuid.UUID
OwnerKind   string
OwnerUserID *uuid.UUID
Provider    string
Name        string
SecretID    *uuid.UUID
KeyPrefix   *string
BaseURL     *string
Model       string
IsDefault   bool
RevokedAt   *time.Time
CreatedAt   time.Time
UpdatedAt   time.Time
// APIKeyLegacy 是 SQLite migration 遗留字段（secret_id 为 nil 时的明文 fallback）
APIKeyLegacy *string
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

func (r *AsrCredentialsRepository) WithTx(tx pgx.Tx) *AsrCredentialsRepository {
return &AsrCredentialsRepository{db: tx}
}

const asrCredCols = `id, owner_kind, owner_user_id, provider, name, secret_id, key_prefix, base_url, model, is_default, revoked_at, created_at, updated_at, api_key_legacy`

func scanAsrCredential(row interface{ Scan(dest ...any) error }) (AsrCredential, error) {
var c AsrCredential
err := row.Scan(
&c.ID, &c.OwnerKind, &c.OwnerUserID, &c.Provider, &c.Name, &c.SecretID, &c.KeyPrefix,
&c.BaseURL, &c.Model, &c.IsDefault, &c.RevokedAt, &c.CreatedAt, &c.UpdatedAt, &c.APIKeyLegacy,
)
return c, err
}

func (r *AsrCredentialsRepository) Create(
ctx context.Context,
id uuid.UUID,
ownerKind string,
ownerUserID *uuid.UUID,
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
if ownerKind != "user" && ownerKind != "platform" {
return AsrCredential{}, fmt.Errorf("owner_kind must be user or platform")
}
if ownerKind == "user" && (ownerUserID == nil || *ownerUserID == uuid.Nil) {
return AsrCredential{}, fmt.Errorf("owner_user_id must not be nil for user owner_kind")
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

c, err := scanAsrCredential(r.db.QueryRow(
ctx,
`INSERT INTO asr_credentials
    (id, owner_kind, owner_user_id, provider, name, secret_id, key_prefix, base_url, model, is_default)
 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
 RETURNING `+asrCredCols,
id, ownerKind, ownerUserID, provider, name, secretID, keyPrefix, baseURL, model, isDefault,
))
if err != nil {
if isUniqueViolation(err) {
return AsrCredential{}, AsrCredentialNameConflictError{Name: name}
}
return AsrCredential{}, err
}
return c, nil
}

func (r *AsrCredentialsRepository) GetByID(ctx context.Context, ownerKind string, ownerUserID *uuid.UUID, id uuid.UUID) (*AsrCredential, error) {
if ctx == nil {
ctx = context.Background()
}
query := `SELECT ` + asrCredCols + `
 FROM asr_credentials
 WHERE id = $1 AND revoked_at IS NULL`
args := []any{id}
query, args = appendOwnerKindFilter(query, args, ownerKind, ownerUserID)

c, err := scanAsrCredential(r.db.QueryRow(ctx, query, args...))
if err != nil {
if errors.Is(err, pgx.ErrNoRows) {
return nil, nil
}
return nil, err
}
return &c, nil
}

// GetDefault 返回 user 级 default，找不到时 fallback 到 platform 级 default。
func (r *AsrCredentialsRepository) GetDefault(ctx context.Context, ownerUserID uuid.UUID) (*AsrCredential, error) {
if ctx == nil {
ctx = context.Background()
}
c, err := scanAsrCredential(r.db.QueryRow(
ctx,
`SELECT `+asrCredCols+`
 FROM asr_credentials
 WHERE owner_kind = 'user' AND owner_user_id = $1 AND is_default = 1 AND revoked_at IS NULL
 LIMIT 1`,
ownerUserID,
))
if err != nil {
if errors.Is(err, pgx.ErrNoRows) {
return nil, nil
}
return nil, err
}
return &c, nil
}

func (r *AsrCredentialsRepository) ListByOwner(ctx context.Context, ownerUserID uuid.UUID) ([]AsrCredential, error) {
if ctx == nil {
ctx = context.Background()
}
rows, err := r.db.Query(
ctx,
`SELECT `+asrCredCols+`
 FROM asr_credentials
 WHERE ((owner_kind = 'user' AND owner_user_id = $1) OR owner_kind = 'platform') AND revoked_at IS NULL
 ORDER BY owner_kind ASC, created_at DESC`,
ownerUserID,
)
if err != nil {
return nil, err
}
defer rows.Close()

creds := []AsrCredential{}
for rows.Next() {
c, err := scanAsrCredential(rows)
if err != nil {
return nil, err
}
creds = append(creds, c)
}
return creds, rows.Err()
}

// SetDefault 原子地将指定 user 的凭证设为 default，同时清除同 user 其他凭证的 default 标记。
func (r *AsrCredentialsRepository) SetDefault(ctx context.Context, ownerUserID uuid.UUID, id uuid.UUID) error {
if ctx == nil {
ctx = context.Background()
}
_, err := r.db.Exec(
ctx,
`UPDATE asr_credentials
 SET is_default = (id = $2), updated_at = now()
 WHERE owner_kind = 'user' AND owner_user_id = $1 AND revoked_at IS NULL`,
ownerUserID, id,
)
return err
}

// SetDefaultPlatform 原子地将 platform 凭证设为 default。
func (r *AsrCredentialsRepository) SetDefaultPlatform(ctx context.Context, id uuid.UUID) error {
if ctx == nil {
ctx = context.Background()
}
_, err := r.db.Exec(
ctx,
`UPDATE asr_credentials
 SET is_default = (id = $1), updated_at = now()
 WHERE owner_kind = 'platform' AND revoked_at IS NULL`,
id,
)
return err
}

func (r *AsrCredentialsRepository) Delete(ctx context.Context, ownerKind string, ownerUserID *uuid.UUID, id uuid.UUID) error {
if ctx == nil {
ctx = context.Background()
}
query := `DELETE FROM asr_credentials WHERE id = $1`
args := []any{id}
query, args = appendOwnerKindFilter(query, args, ownerKind, ownerUserID)
_, err := r.db.Exec(ctx, query, args...)
return err
}

func (r *AsrCredentialsRepository) Update(
ctx context.Context,
ownerKind string,
ownerUserID *uuid.UUID,
id uuid.UUID,
name *string,
baseURL *string,
model *string,
isDefault *bool,
) error {
if ctx == nil {
ctx = context.Background()
}
setClauses := []string{}
args := []any{}
argIdx := 1

if name != nil {
setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
args = append(args, *name)
argIdx++
}
if baseURL != nil {
setClauses = append(setClauses, fmt.Sprintf("base_url = $%d", argIdx))
args = append(args, *baseURL)
argIdx++
}
if model != nil {
setClauses = append(setClauses, fmt.Sprintf("model = $%d", argIdx))
args = append(args, *model)
argIdx++
}
if isDefault != nil {
setClauses = append(setClauses, fmt.Sprintf("is_default = $%d", argIdx))
args = append(args, *isDefault)
argIdx++
}

if len(setClauses) == 0 {
return nil
}

args = append(args, id)
query := fmt.Sprintf(
"UPDATE asr_credentials SET %s, updated_at = datetime('now') WHERE id = $%d",
strings.Join(setClauses, ", "),
argIdx,
)
query, args = appendOwnerKindFilter(query, args, ownerKind, ownerUserID)
_, err := r.db.Exec(ctx, query, args...)
return err
}
