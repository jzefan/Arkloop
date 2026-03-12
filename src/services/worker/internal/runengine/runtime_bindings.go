package runengine

import (
	"context"

	"arkloop/services/shared/database"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/environmentbindings"
)

func resolveAndPersistEnvironmentBindings(ctx context.Context, db database.DB, run data.Run) (data.Run, error) {
	return environmentbindings.ResolveAndPersistRun(ctx, db, run)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
