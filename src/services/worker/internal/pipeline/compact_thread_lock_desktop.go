//go:build desktop

package pipeline

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func compactThreadCompactionAdvisoryXactLock(_ context.Context, _ pgx.Tx, _ uuid.UUID) error {
	return nil
}
