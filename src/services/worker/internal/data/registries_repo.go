package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	FlushStateIdle    = "idle"
	FlushStatePending = "pending"
	FlushStateRunning = "running"
	FlushStateFailed  = "failed"
)

type RegistryRecord struct {
	Ref                  string
	OrgID                uuid.UUID
	LatestManifestRev    *string
	FlushState           string
	FlushRetryCount      int
	LastFlushFailedAt    *time.Time
	LastFlushSucceededAt *time.Time
	MetadataJSON         map[string]any
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ProfileRegistriesRepository struct{}

type WorkspaceRegistriesRepository struct{}

func (ProfileRegistriesRepository) Get(ctx context.Context, pool *pgxpool.Pool, profileRef string) (RegistryRecord, error) {
	return getRegistry(ctx, pool, "profile_registries", "profile_ref", profileRef)
}

func (WorkspaceRegistriesRepository) Get(ctx context.Context, pool *pgxpool.Pool, workspaceRef string) (RegistryRecord, error) {
	return getRegistry(ctx, pool, "workspace_registries", "workspace_ref", workspaceRef)
}

func (ProfileRegistriesRepository) GetOrCreate(ctx context.Context, pool *pgxpool.Pool, record RegistryRecord) (RegistryRecord, error) {
	return getOrCreateRegistry(ctx, pool, "profile_registries", "profile_ref", record)
}

func (WorkspaceRegistriesRepository) GetOrCreate(ctx context.Context, pool *pgxpool.Pool, record RegistryRecord) (RegistryRecord, error) {
	return getOrCreateRegistry(ctx, pool, "workspace_registries", "workspace_ref", record)
}

func (ProfileRegistriesRepository) MarkFlushPending(ctx context.Context, pool *pgxpool.Pool, profileRef string) error {
	return markRegistryFlushPending(ctx, pool, "profile_registries", "profile_ref", profileRef)
}

func (WorkspaceRegistriesRepository) MarkFlushPending(ctx context.Context, pool *pgxpool.Pool, workspaceRef string) error {
	return markRegistryFlushPending(ctx, pool, "workspace_registries", "workspace_ref", workspaceRef)
}

func (ProfileRegistriesRepository) MarkFlushRunning(ctx context.Context, pool *pgxpool.Pool, profileRef string) error {
	return markRegistryFlushRunning(ctx, pool, "profile_registries", "profile_ref", profileRef)
}

func (WorkspaceRegistriesRepository) MarkFlushRunning(ctx context.Context, pool *pgxpool.Pool, workspaceRef string) error {
	return markRegistryFlushRunning(ctx, pool, "workspace_registries", "workspace_ref", workspaceRef)
}

func (ProfileRegistriesRepository) MarkFlushFailed(ctx context.Context, pool *pgxpool.Pool, profileRef string, failedAt time.Time) error {
	return markRegistryFlushFailed(ctx, pool, "profile_registries", "profile_ref", profileRef, failedAt)
}

func (WorkspaceRegistriesRepository) MarkFlushFailed(ctx context.Context, pool *pgxpool.Pool, workspaceRef string, failedAt time.Time) error {
	return markRegistryFlushFailed(ctx, pool, "workspace_registries", "workspace_ref", workspaceRef, failedAt)
}

func (ProfileRegistriesRepository) MarkFlushSucceeded(ctx context.Context, pool *pgxpool.Pool, profileRef string, revision string, succeededAt time.Time) error {
	return markRegistryFlushSucceeded(ctx, pool, "profile_registries", "profile_ref", profileRef, revision, succeededAt)
}

func (WorkspaceRegistriesRepository) MarkFlushSucceeded(ctx context.Context, pool *pgxpool.Pool, workspaceRef string, revision string, succeededAt time.Time) error {
	return markRegistryFlushSucceeded(ctx, pool, "workspace_registries", "workspace_ref", workspaceRef, revision, succeededAt)
}

func getRegistry(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, ref string) (RegistryRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		return RegistryRecord{}, fmt.Errorf("pool must not be nil")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return RegistryRecord{}, fmt.Errorf("registry ref must not be empty")
	}

	var record RegistryRecord
	var metadataRaw []byte
	err := pool.QueryRow(
		ctx,
		fmt.Sprintf(`SELECT %s,
			       org_id,
			       latest_manifest_rev,
			       flush_state,
			       flush_retry_count,
			       last_flush_failed_at,
			       last_flush_succeeded_at,
			       metadata_json,
			       created_at,
			       updated_at
		   FROM %s
		  WHERE %s = $1`, keyColumn, table, keyColumn),
		ref,
	).Scan(
		&record.Ref,
		&record.OrgID,
		&record.LatestManifestRev,
		&record.FlushState,
		&record.FlushRetryCount,
		&record.LastFlushFailedAt,
		&record.LastFlushSucceededAt,
		&metadataRaw,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return RegistryRecord{}, err
	}
	if len(metadataRaw) > 0 {
		_ = json.Unmarshal(metadataRaw, &record.MetadataJSON)
	}
	if record.MetadataJSON == nil {
		record.MetadataJSON = map[string]any{}
	}
	record.FlushState = normalizeFlushState(record.FlushState)
	return record, nil
}

func getOrCreateRegistry(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, record RegistryRecord) (RegistryRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		return RegistryRecord{}, fmt.Errorf("pool must not be nil")
	}
	if record.OrgID == uuid.Nil {
		return RegistryRecord{}, fmt.Errorf("org_id must not be empty")
	}
	record.Ref = strings.TrimSpace(record.Ref)
	if record.Ref == "" {
		return RegistryRecord{}, fmt.Errorf("registry ref must not be empty")
	}
	record.FlushState = normalizeFlushState(record.FlushState)
	if record.MetadataJSON == nil {
		record.MetadataJSON = map[string]any{}
	}
	metadataRaw, err := json.Marshal(record.MetadataJSON)
	if err != nil {
		return RegistryRecord{}, fmt.Errorf("marshal metadata_json: %w", err)
	}

	_, err = pool.Exec(
		ctx,
		fmt.Sprintf(`INSERT INTO %s (
			%s,
			org_id,
			latest_manifest_rev,
			flush_state,
			flush_retry_count,
			metadata_json
		) VALUES ($1, $2, $3, $4, 0, $5::jsonb)
		ON CONFLICT (%s) DO NOTHING`, table, keyColumn, keyColumn),
		record.Ref,
		record.OrgID,
		record.LatestManifestRev,
		record.FlushState,
		string(metadataRaw),
	)
	if err != nil {
		return RegistryRecord{}, err
	}
	return getRegistry(ctx, pool, table, keyColumn, record.Ref)
}

func markRegistryFlushPending(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, ref string) error {
	return updateRegistryState(ctx, pool, table, keyColumn, ref, `
		flush_state = 'pending',
		updated_at = now()`)
}

func markRegistryFlushRunning(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, ref string) error {
	return updateRegistryState(ctx, pool, table, keyColumn, ref, `
		flush_state = 'running',
		updated_at = now()`)
}

func markRegistryFlushFailed(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, ref string, failedAt time.Time) error {
	if failedAt.IsZero() {
		failedAt = time.Now().UTC()
	}
	return updateRegistryState(ctx, pool, table, keyColumn, ref, `
		flush_state = 'failed',
		flush_retry_count = flush_retry_count + 1,
		last_flush_failed_at = $2,
		updated_at = now()`, failedAt.UTC())
}

func markRegistryFlushSucceeded(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, ref string, revision string, succeededAt time.Time) error {
	if succeededAt.IsZero() {
		succeededAt = time.Now().UTC()
	}
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return fmt.Errorf("latest manifest revision must not be empty")
	}
	return updateRegistryState(ctx, pool, table, keyColumn, ref, `
		latest_manifest_rev = $2,
		flush_state = 'idle',
		flush_retry_count = 0,
		last_flush_succeeded_at = $3,
		updated_at = now()`, revision, succeededAt.UTC())
}

func updateRegistryState(ctx context.Context, pool *pgxpool.Pool, table string, keyColumn string, ref string, setClause string, args ...any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		return fmt.Errorf("pool must not be nil")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("registry ref must not be empty")
	}
	queryArgs := make([]any, 0, len(args)+1)
	queryArgs = append(queryArgs, ref)
	queryArgs = append(queryArgs, args...)
	commandTag, err := pool.Exec(
		ctx,
		fmt.Sprintf(`UPDATE %s
		    SET %s
		  WHERE %s = $1`, table, strings.TrimSpace(setClause), keyColumn),
		queryArgs...,
	)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func normalizeFlushState(value string) string {
	switch strings.TrimSpace(value) {
	case FlushStatePending, FlushStateRunning, FlushStateFailed:
		return strings.TrimSpace(value)
	default:
		return FlushStateIdle
	}
}

func IsRegistryNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
