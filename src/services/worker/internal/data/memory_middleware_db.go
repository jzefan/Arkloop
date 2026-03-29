package data

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MemoryMiddlewareDB 由 Postgres 池与桌面 SQLite 实现，供 memory 中间件写 usage / run_events。
type MemoryMiddlewareDB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}
