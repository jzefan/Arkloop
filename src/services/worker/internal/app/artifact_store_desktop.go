//go:build desktop

package app

import (
	"context"

	"arkloop/services/shared/desktop"
	"arkloop/services/shared/objectstore"
)

func openDesktopArtifactStore(ctx context.Context) (objectstore.Store, error) {
	dataDir, err := desktop.ResolveDataDir("")
	if err != nil {
		return nil, err
	}
	return objectstore.NewFilesystemOpener(desktop.StorageRoot(dataDir)).Open(ctx, objectstore.ArtifactBucket)
}
