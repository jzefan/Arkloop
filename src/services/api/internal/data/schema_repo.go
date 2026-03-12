package data

import (
	"context"
	"fmt"

	"arkloop/services/shared/database"
)

type SchemaRepository struct {
	db database.DB
}

func NewSchemaRepository(db database.DB) (*SchemaRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	return &SchemaRepository{db: db}, nil
}

func (r *SchemaRepository) CurrentSchemaVersion(ctx context.Context) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var version int64
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = true`,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query goose_db_version: %w", err)
	}
	return version, nil
}
