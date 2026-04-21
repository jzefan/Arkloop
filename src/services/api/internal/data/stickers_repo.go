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

type AccountSticker struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	ContentHash       string
	StorageKey        string
	PreviewStorageKey string
	FileSize          int64
	MimeType          string
	IsAnimated        bool
	ShortTags         string
	LongDesc          string
	UsageCount        int
	LastUsedAt        *time.Time
	IsRegistered      bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type StickerDescriptionCache struct {
	ContentHash string
	Description string
	EmotionTags string
	Timestamp   time.Time
}

type AccountStickerUpsert struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	ContentHash       string
	StorageKey        string
	PreviewStorageKey string
	FileSize          int64
	MimeType          string
	IsAnimated        bool
}

type AccountStickersRepository struct {
	db Querier
}

func NewAccountStickersRepository(db Querier) (*AccountStickersRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &AccountStickersRepository{db: db}, nil
}

func (r *AccountStickersRepository) WithTx(tx pgx.Tx) *AccountStickersRepository {
	return &AccountStickersRepository{db: tx}
}

func scanAccountSticker(row interface{ Scan(dest ...any) error }) (AccountSticker, error) {
	var item AccountSticker
	err := row.Scan(
		&item.ID,
		&item.AccountID,
		&item.ContentHash,
		&item.StorageKey,
		&item.PreviewStorageKey,
		&item.FileSize,
		&item.MimeType,
		&item.IsAnimated,
		&item.ShortTags,
		&item.LongDesc,
		&item.UsageCount,
		&item.LastUsedAt,
		&item.IsRegistered,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *AccountStickersRepository) GetByHash(ctx context.Context, accountID uuid.UUID, contentHash string) (*AccountSticker, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	var item AccountSticker
	err := r.db.QueryRow(ctx, `
		SELECT id, account_id, content_hash, storage_key, preview_storage_key, file_size, mime_type,
		       is_animated, short_tags, long_desc, usage_count, last_used_at, is_registered, created_at, updated_at
		  FROM account_stickers
		 WHERE account_id = $1
		   AND content_hash = $2`,
		accountID, strings.TrimSpace(contentHash),
	).Scan(
		&item.ID,
		&item.AccountID,
		&item.ContentHash,
		&item.StorageKey,
		&item.PreviewStorageKey,
		&item.FileSize,
		&item.MimeType,
		&item.IsAnimated,
		&item.ShortTags,
		&item.LongDesc,
		&item.UsageCount,
		&item.LastUsedAt,
		&item.IsRegistered,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *AccountStickersRepository) UpsertPending(ctx context.Context, item AccountStickerUpsert) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("db must not be nil")
	}
	now := time.Now().UTC()
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO account_stickers (
			id, account_id, content_hash, storage_key, preview_storage_key, file_size, mime_type,
			is_animated, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $9
		)
		ON CONFLICT (account_id, content_hash) DO UPDATE SET
			storage_key = EXCLUDED.storage_key,
			preview_storage_key = EXCLUDED.preview_storage_key,
			file_size = EXCLUDED.file_size,
			mime_type = EXCLUDED.mime_type,
			is_animated = EXCLUDED.is_animated,
			updated_at = EXCLUDED.updated_at`,
		item.ID,
		item.AccountID,
		strings.TrimSpace(item.ContentHash),
		strings.TrimSpace(item.StorageKey),
		strings.TrimSpace(item.PreviewStorageKey),
		item.FileSize,
		strings.TrimSpace(item.MimeType),
		item.IsAnimated,
		now,
	)
	return err
}

func (r *AccountStickersRepository) MarkRegistered(ctx context.Context, accountID uuid.UUID, contentHash, description, tags string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("db must not be nil")
	}
	now := time.Now().UTC()
	_, err := r.db.Exec(ctx, `
		UPDATE account_stickers
		   SET long_desc = $3,
		       short_tags = $4,
		       is_registered = TRUE,
		       updated_at = $5
		 WHERE account_id = $1
		   AND content_hash = $2`,
		accountID,
		strings.TrimSpace(contentHash),
		strings.TrimSpace(description),
		strings.TrimSpace(tags),
		now,
	)
	return err
}

type StickerDescriptionCacheRepository struct {
	db Querier
}

func NewStickerDescriptionCacheRepository(db Querier) (*StickerDescriptionCacheRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &StickerDescriptionCacheRepository{db: db}, nil
}

func (r *StickerDescriptionCacheRepository) WithTx(tx pgx.Tx) *StickerDescriptionCacheRepository {
	return &StickerDescriptionCacheRepository{db: tx}
}

func (r *StickerDescriptionCacheRepository) Get(ctx context.Context, contentHash string) (*StickerDescriptionCache, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	var item StickerDescriptionCache
	err := r.db.QueryRow(ctx, `
		SELECT content_hash, description, emotion_tags, timestamp
		  FROM sticker_description_cache
		 WHERE content_hash = $1`,
		strings.TrimSpace(contentHash),
	).Scan(&item.ContentHash, &item.Description, &item.EmotionTags, &item.Timestamp)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *StickerDescriptionCacheRepository) Upsert(ctx context.Context, contentHash, description, tags string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("db must not be nil")
	}
	now := time.Now().UTC()
	_, err := r.db.Exec(ctx, `
		INSERT INTO sticker_description_cache (content_hash, description, emotion_tags, timestamp)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (content_hash) DO UPDATE SET
			description = EXCLUDED.description,
			emotion_tags = EXCLUDED.emotion_tags,
			timestamp = EXCLUDED.timestamp`,
		strings.TrimSpace(contentHash),
		strings.TrimSpace(description),
		strings.TrimSpace(tags),
		now,
	)
	return err
}
