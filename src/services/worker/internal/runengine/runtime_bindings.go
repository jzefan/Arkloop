package runengine

import (
	"context"

	"arkloop/services/shared/database"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/environmentbindings"
)

func resolveAndPersistEnvironmentBindings(ctx context.Context, db database.DB, run data.Run, dialect database.DialectHelper) (data.Run, error) {
	return environmentbindings.ResolveAndPersistRun(ctx, db, run, dialect)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
