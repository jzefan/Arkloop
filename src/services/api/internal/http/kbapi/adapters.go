package kbapi

import (
	"context"

	"arkloop/services/api/internal/data"
	"arkloop/services/shared/objectstore"

	"github.com/google/uuid"
)

type WorkspaceMembership struct {
	Workspaces *data.WorkspaceRegistriesRepository
}

func (m *WorkspaceMembership) IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, workspaceRef string) (bool, error) {
	if m == nil || m.Workspaces == nil {
		return false, nil
	}
	ws, err := m.Workspaces.Get(ctx, workspaceRef)
	if err != nil || ws == nil {
		return false, err
	}
	return ws.AccountID == accountID, nil
}

type WorkspaceBlobAdapter struct {
	Store objectstore.Store
}

func (b *WorkspaceBlobAdapter) PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error {
	return b.Store.Put(ctx, kbBlobKey(workspaceRef, sha256), data)
}

func kbBlobKey(workspaceRef, sha256 string) string {
	return "workspaces/" + workspaceRef + "/kb/blobs/" + sha256
}
