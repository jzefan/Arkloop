package config

import (
	"context"
	"fmt"

	"arkloop/services/shared/database"

	"github.com/google/uuid"
)

type Store interface {
	GetPlatformSetting(ctx context.Context, key string) (string, bool, error)
	GetOrgSetting(ctx context.Context, orgID uuid.UUID, key string) (string, bool, error)
}

type DBStore struct {
	db database.DB
}

func NewDBStore(db database.DB) *DBStore {
	return &DBStore{db: db}
}

// NewPGXStore is a backward-compatible alias that accepts database.DB.
// Deprecated: use NewDBStore instead.
func NewPGXStore(db database.DB) *DBStore {
	return NewDBStore(db)
}

func (s *DBStore) GetPlatformSetting(ctx context.Context, key string) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.db == nil {
		return "", false, nil
	}

	var value string
	err := s.db.QueryRow(ctx, `SELECT value FROM platform_settings WHERE key = $1 LIMIT 1`, key).Scan(&value)
	if database.IsNoRows(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get platform setting %q: %w", key, err)
	}
	return value, true, nil
}

func (s *DBStore) GetOrgSetting(ctx context.Context, orgID uuid.UUID, key string) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.db == nil {
		return "", false, nil
	}
	if orgID == uuid.Nil {
		return "", false, fmt.Errorf("org_id must not be empty")
	}

	var value string
	err := s.db.QueryRow(ctx, `SELECT value FROM org_settings WHERE org_id = $1 AND key = $2 LIMIT 1`, orgID, key).Scan(&value)
	if database.IsNoRows(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get org setting %q: %w", key, err)
	}
	return value, true, nil
}
