package database

import "context"

// DB abstracts a database connection pool.
// Both PostgreSQL (pgxpool.Pool) and SQLite can implement this interface.
type DB interface {
	Querier
	Begin(ctx context.Context) (Tx, error)
	Close() error
	Ping(ctx context.Context) error
}

// Tx abstracts a database transaction.
type Tx interface {
	Querier
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// Querier is the shared query interface satisfied by both DB and Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

// Row abstracts a single-row query result.
type Row interface {
	Scan(dest ...any) error
}

// Rows abstracts a multi-row query result set.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// Result abstracts the outcome of an Exec operation.
type Result interface {
	RowsAffected() int64
}
