// Package kbapi serves M1.0 knowledge-base endpoints. Auth follows the
// shared pattern: httpkit.ResolveActor handles user→account, then each
// route checks workspace membership for the requested kb.workspace_ref.
package kbapi

import (
	"context"

	"github.com/google/uuid"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/shared/embedding"
	"arkloop/services/shared/objectstore"
)

// KBIngestEnqueuer is the narrow surface kbapi uses to push a kb_ingest
// job onto the queue. Task 9 supplies a concrete wrapper around the
// worker's JobQueue (declared in worker/internal/queue) so the api
// package does not need to import worker's internals.
type KBIngestEnqueuer interface {
	EnqueueKBIngest(
		ctx context.Context,
		accountID, kbID, docID uuid.UUID,
		workspaceRef, blobSHA256, mimeType, filename, traceID string,
	) (uuid.UUID, error)
}

type Deps struct {
	AuthService           *auth.Service
	AccountMembershipRepo *data.AccountMembershipRepository
	APIKeysRepo           *data.APIKeysRepository
	AuditWriter           *audit.Writer

	Pool                    data.DB
	KnowledgeBasesRepo      *data.KnowledgeBasesRepository
	KBDocumentsRepo         *data.KBDocumentsRepository
	KBChunksRepo            *data.KBChunksRepository
	ProfileRegistriesRepo   *data.ProfileRegistriesRepository
	WorkspaceRegistriesRepo *data.WorkspaceRegistriesRepository

	BlobStore objectstore.Store
	Embedder  embedding.Embedder // for the search REST endpoint

	JobEnqueuer KBIngestEnqueuer // for enqueuing kb_ingest

	// Limits
	MaxUploadBytes int64 // 10 MB default
}
