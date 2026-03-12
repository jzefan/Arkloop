package database

import (
	"context"
	"io/fs"
)

// Migrator abstracts database schema migration.
type Migrator interface {
	// Up applies all pending migrations and returns the count of applied migrations.
	Up(ctx context.Context) (int, error)

	// Down rolls back the most recent migration.
	Down(ctx context.Context) error

	// CurrentVersion returns the current schema version.
	CurrentVersion(ctx context.Context) (int64, error)
}

// MigratorFactory creates a Migrator for a given DB and migration source.
type MigratorFactory interface {
	NewMigrator(db DB, dialect Dialect, migrations fs.FS) (Migrator, error)
}
