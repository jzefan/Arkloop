package kbingest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"arkloop/services/shared/bookparser"
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
	metas    []map[string]any
}

func (f *fakeChunksRepo) Upsert(ctx context.Context, rows []data.KBChunkUpsert) error {
	f.upserted = append(f.upserted, rows...)
	return nil
}

func (f *fakeChunksRepo) UpdateDocStatus(ctx context.Context, id uuid.UUID, status, msg string, parseMeta map[string]any) error {
	f.statuses = append(f.statuses, status)
	f.errors = append(f.errors, msg)
	f.metas = append(f.metas, parseMeta)
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

type fakeParser struct {
	doc bookparser.ParsedDoc
}

func (p fakeParser) Parse(ctx context.Context, r io.Reader, mime string) (bookparser.ParsedDoc, error) {
	buf := &bytes.Buffer{}
	_, _ = buf.ReadFrom(r)
	return p.doc, nil
}

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

func TestProcessorPersistsParseAndChunkMetadata(t *testing.T) {
	docID := uuid.New()
	kbID := uuid.New()
	blob := &fakeBlobReader{content: map[string][]byte{"ws/abc": []byte("fake pdf")}}
	chunks := &fakeChunksRepo{}
	parser := fakeParser{doc: bookparser.ParsedDoc{
		Meta: map[string]any{
			"heading_inferred_ratio": 0.8,
			"block_type_counts":      map[string]any{"image": 1},
		},
		Blocks: []bookparser.Block{
			{
				Type:        bookparser.BlockImage,
				Text:        "[Image: 光路图]",
				HeadingPath: []string{"第一章"},
				Metadata:    map[string]any{"page": 4, "asset_sha256": "abc"},
			},
		},
	}}
	p, err := NewProcessorWithParser(blob, chunks, fakeEmb{dim: 8}, parser)
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
			"mime_type":         "application/pdf",
			"original_filename": "a.pdf",
		},
	}
	if err := p.Handle(context.Background(), lease); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(chunks.upserted) != 1 {
		t.Fatalf("upserted: got %d, want 1", len(chunks.upserted))
	}
	if chunks.upserted[0].ChunkType != "image" || chunks.upserted[0].Metadata["asset_sha256"] != "abc" {
		t.Fatalf("chunk metadata not preserved: %+v", chunks.upserted[0])
	}
	foundMeta := false
	for _, meta := range chunks.metas {
		if meta != nil && meta["heading_inferred_ratio"] == 0.8 {
			foundMeta = true
			break
		}
	}
	if !foundMeta {
		t.Fatalf("parse meta not persisted through statuses: %+v", chunks.metas)
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
