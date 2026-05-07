//go:build !desktop

package pipeline

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func compactThreadCompactionAdvisoryXactLock(ctx context.Context, tx pgx.Tx, threadID uuid.UUID) error {
	if tx == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text)::bigint)`, threadID.String())
	return err
}
