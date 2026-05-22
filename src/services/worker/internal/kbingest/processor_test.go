package kbingest

import (
	"context"
	"errors"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/queue"

	"github.com/google/uuid"
)

type fakeBlobReader struct {
	content map[string][]byte
}

func (f *fakeBlobReader) GetBlob(ctx context.Context, workspaceRef, sha string) ([]byte, error) {
	v, ok := f.content[workspaceRef+"/"+sha]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

type fakeChunksRepo struct {
	upserted []data.KBChunkUpsert
	statuses []string
	errors   []string
}

func (f *fakeChunksRepo) Upsert(ctx context.Context, rows []data.KBChunkUpsert) error {
	f.upserted = append(f.upserted, rows...)
	return nil
}

func (f *fakeChunksRepo) UpdateDocStatus(ctx context.Context, id uuid.UUID, status, msg string) error {
	f.statuses = append(f.statuses, status)
	f.errors = append(f.errors, msg)
	return nil
}

func (f *fakeChunksRepo) Dim() int { return 8 }

type fakeEmb struct {
	dim int
}

func (e fakeEmb) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, e.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}

func (e fakeEmb) Dim() int { return e.dim }

func TestProcessorHappyPath(t *testing.T) {
	docID := uuid.New()
	kbID := uuid.New()
	blob := &fakeBlobReader{content: map[string][]byte{"ws/abc": []byte("段落 A 内容。\n\n段落 B 内容。")}}
	chunks := &fakeChunksRepo{}
	p, err := NewProcessor(blob, chunks, fakeEmb{dim: 8})
	if err != nil {
		t.Fatal(err)
	}
	lease := queue.JobLease{
		JobID: uuid.New(),
		PayloadJSON: map[string]any{
			"type":              queue.KBIngestJobType,
			"kb_id":             kbID.String(),
			"document_id":       docID.String(),
			"workspace_ref":     "ws",
			"blob_sha256":       "abc",
			"mime_type":         "text/plain",
			"original_filename": "a.txt",
		},
	}
	if err := p.Handle(context.Background(), lease); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(chunks.upserted) != 2 {
		t.Errorf("upserted: got %d, want 2", len(chunks.upserted))
	}
	wantStatuses := []string{"parsing", "chunking", "embedding", "upserting", "ready"}
	if len(chunks.statuses) != len(wantStatuses) {
		t.Fatalf("statuses: got %v, want %v", chunks.statuses, wantStatuses)
	}
	for i, s := range wantStatuses {
		if chunks.statuses[i] != s {
			t.Errorf("status %d: got %q, want %q", i, chunks.statuses[i], s)
		}
	}
}

func TestProcessorMarksFailedOnBlobError(t *testing.T) {
	docID := uuid.New()
	blob := &fakeBlobReader{content: map[string][]byte{}}
	chunks := &fakeChunksRepo{}
	p, err := NewProcessor(blob, chunks, fakeEmb{dim: 8})
	if err != nil {
		t.Fatal(err)
	}
	lease := queue.JobLease{
		JobID: uuid.New(),
		PayloadJSON: map[string]any{
			"type":              queue.KBIngestJobType,
			"kb_id":             uuid.New().String(),
			"document_id":       docID.String(),
			"workspace_ref":     "ws",
			"blob_sha256":       "missing",
			"mime_type":         "text/plain",
			"original_filename": "a.txt",
		},
	}
	if err := p.Handle(context.Background(), lease); err == nil {
		t.Error("expected error")
	}
	last := chunks.statuses[len(chunks.statuses)-1]
	if last != "failed" {
		t.Errorf("last status: got %q, want failed", last)
	}
}
