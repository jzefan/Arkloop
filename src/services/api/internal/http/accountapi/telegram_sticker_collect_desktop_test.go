//go:build desktop

package accountapi

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"arkloop/services/api/internal/data"
	"arkloop/services/shared/database/sqliteadapter"
	"arkloop/services/shared/database/sqlitepgx"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestUpsertTelegramStickerPendingTx_NewStickerTriggersRegister(t *testing.T) {
	ctx := context.Background()
	pool, accountID := openStickerCollectDesktopDB(t, ctx)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	sticker, shouldRegister, err := upsertTelegramStickerPendingTx(ctx, tx, accountID, telegramCollectedSticker{
		ContentHash:       "hash-new",
		StorageKey:        "raw.webp",
		PreviewStorageKey: "preview.jpg",
		FileSize:          128,
		MimeType:          "image/webp",
	})
	if err != nil {
		t.Fatalf("upsert pending: %v", err)
	}
	if !shouldRegister {
		t.Fatal("expected new sticker to trigger register run")
	}
	if sticker == nil || sticker.ContentHash != "hash-new" {
		t.Fatalf("unexpected sticker row: %#v", sticker)
	}
}

func TestUpsertTelegramStickerPendingTx_PendingWithinWindowDoesNotRetrigger(t *testing.T) {
	ctx := context.Background()
	pool, accountID := openStickerCollectDesktopDB(t, ctx)
	seedStickerRow(t, ctx, pool, accountID, "hash-window", time.Now().UTC().Add(-30*time.Minute))

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	sticker, shouldRegister, err := upsertTelegramStickerPendingTx(ctx, tx, accountID, telegramCollectedSticker{
		ContentHash:       "hash-window",
		StorageKey:        "raw-new.webp",
		PreviewStorageKey: "preview-new.jpg",
		FileSize:          256,
		MimeType:          "image/webp",
	})
	if err != nil {
		t.Fatalf("upsert pending: %v", err)
	}
	if shouldRegister {
		t.Fatal("expected pending sticker inside retry window to skip register run")
	}
	if sticker == nil || sticker.StorageKey != "raw-new.webp" || sticker.PreviewStorageKey != "preview-new.jpg" {
		t.Fatalf("expected metadata refresh on pending sticker, got %#v", sticker)
	}

	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT updated_at
		  FROM account_stickers
		 WHERE account_id = $1
		   AND content_hash = $2`,
		accountID, "hash-window",
	).Scan(&updatedAt); err != nil {
		t.Fatalf("query updated_at: %v", err)
	}
	if updatedAt.After(time.Now().UTC().Add(-20 * time.Minute)) {
		t.Fatalf("expected updated_at to stay inside original retry window, got %s", updatedAt)
	}
}

func TestUpsertTelegramStickerPendingTx_StalePendingClaimsRetry(t *testing.T) {
	ctx := context.Background()
	pool, accountID := openStickerCollectDesktopDB(t, ctx)
	seedStickerRow(t, ctx, pool, accountID, "hash-stale", time.Now().UTC().Add(-2*time.Hour))

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, shouldRegister, err := upsertTelegramStickerPendingTx(ctx, tx, accountID, telegramCollectedSticker{
		ContentHash:       "hash-stale",
		StorageKey:        "raw-retry.webp",
		PreviewStorageKey: "preview-retry.jpg",
		FileSize:          512,
		MimeType:          "image/webp",
	})
	if err != nil {
		t.Fatalf("upsert pending: %v", err)
	}
	if !shouldRegister {
		t.Fatal("expected stale pending sticker to claim a retry")
	}

	var updatedAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT updated_at
		  FROM account_stickers
		 WHERE account_id = $1
		   AND content_hash = $2`,
		accountID, "hash-stale",
	).Scan(&updatedAt); err != nil {
		t.Fatalf("query updated_at: %v", err)
	}
	if updatedAt.Before(time.Now().UTC().Add(-5 * time.Minute)) {
		t.Fatalf("expected retry claim to refresh updated_at, got %s", updatedAt)
	}
}

func TestUpsertTelegramStickerPendingTx_GainingPreviewTriggersRegister(t *testing.T) {
	ctx := context.Background()
	pool, accountID := openStickerCollectDesktopDB(t, ctx)
	seedStickerRowWithoutPreview(t, ctx, pool, accountID, "hash-preview-late", time.Now().UTC().Add(-30*time.Minute))

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	sticker, shouldRegister, err := upsertTelegramStickerPendingTx(ctx, tx, accountID, telegramCollectedSticker{
		ContentHash:       "hash-preview-late",
		StorageKey:        "raw-late.webp",
		PreviewStorageKey: "preview-late.jpg",
		FileSize:          333,
		MimeType:          "image/webp",
	})
	if err != nil {
		t.Fatalf("upsert pending: %v", err)
	}
	if !shouldRegister {
		t.Fatal("expected preview arrival to trigger register run immediately")
	}
	if sticker == nil || sticker.PreviewStorageKey != "preview-late.jpg" {
		t.Fatalf("expected preview key updated, got %#v", sticker)
	}
}

func openStickerCollectDesktopDB(t *testing.T, ctx context.Context) (*sqlitepgx.Pool, uuid.UUID) {
	t.Helper()

	sqlitePool, err := sqliteadapter.AutoMigrate(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}
	pool := sqlitepgx.New(sqlitePool.Unwrap())

	accountRepo, err := data.NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("new account repo: %v", err)
	}
	account, err := accountRepo.Create(ctx, "stickers-"+uuid.NewString(), "stickers", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return pool, account.ID
}

func seedStickerRow(t *testing.T, ctx context.Context, pool *sqlitepgx.Pool, accountID uuid.UUID, contentHash string, updatedAt time.Time) {
	t.Helper()

	createdAt := updatedAt.Add(-time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO account_stickers (
			id, account_id, content_hash, storage_key, preview_storage_key, file_size, mime_type,
			is_animated, short_tags, long_desc, usage_count, is_registered, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, FALSE, '', '', 0, FALSE, $8, $9
		)`,
		uuid.New(),
		accountID,
		contentHash,
		"raw.webp",
		"preview.jpg",
		int64(64),
		"image/webp",
		createdAt.UTC(),
		updatedAt.UTC(),
	); err != nil {
		t.Fatalf("seed sticker row: %v", err)
	}
}

func seedStickerRowWithoutPreview(t *testing.T, ctx context.Context, pool *sqlitepgx.Pool, accountID uuid.UUID, contentHash string, updatedAt time.Time) {
	t.Helper()

	createdAt := updatedAt.Add(-time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO account_stickers (
			id, account_id, content_hash, storage_key, preview_storage_key, file_size, mime_type,
			is_animated, short_tags, long_desc, usage_count, is_registered, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, '', $5, $6, FALSE, '', '', 0, FALSE, $7, $8
		)`,
		uuid.New(),
		accountID,
		contentHash,
		"raw.webp",
		int64(64),
		"image/webp",
		createdAt.UTC(),
		updatedAt.UTC(),
	); err != nil {
		t.Fatalf("seed sticker row without preview: %v", err)
	}
}
