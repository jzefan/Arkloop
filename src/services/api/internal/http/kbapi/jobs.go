package kbapi

import (
	"context"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type JobQueueEnqueuer struct {
	Repo *data.JobRepository
}

func (j *JobQueueEnqueuer) EnqueueKBIngest(ctx context.Context, accountID, kbID, docID uuid.UUID, workspaceRef, blobSHA256, mimeType, filename, traceID string) (uuid.UUID, error) {
	payload := map[string]any{
		"type":              data.KBIngestJobType,
		"kb_id":             kbID.String(),
		"document_id":       docID.String(),
		"workspace_ref":     workspaceRef,
		"blob_sha256":       blobSHA256,
		"mime_type":         mimeType,
		"original_filename": filename,
		"version":           1,
	}
	return j.Repo.EnqueueRun(ctx, accountID, uuid.Nil, traceID, data.KBIngestJobType, payload, nil)
}
