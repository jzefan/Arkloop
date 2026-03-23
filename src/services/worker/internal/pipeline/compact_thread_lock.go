//go:build !desktop

package pipeline

import (
	"context"

	"github.com/google/uuid"
)

// CompactThreadCompactionLock is a no-op on non-desktop builds.
// For PostgreSQL, advisory locks inside the transaction provide the necessary protection.
func CompactThreadCompactionLock(ctx context.Context, threadID uuid.UUID) (func(), error) {
	return func() {}, nil
}
