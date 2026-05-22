package kbingest

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"arkloop/services/shared/bookchunker"
	"arkloop/services/shared/bookparser"
	"arkloop/services/shared/embedding"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/queue"

	"github.com/google/uuid"
)

type BlobReader interface {
	GetBlob(ctx context.Context, workspaceRef, sha256 string) ([]byte, error)
}

type ChunksRepo interface {
	Dim() int
	Upsert(ctx context.Context, rows []data.KBChunkUpsert) error
	UpdateDocStatus(ctx context.Context, id uuid.UUID, status, msg string) error
}

type Processor struct {
	blob     BlobReader
	chunks   ChunksRepo
	embedder embedding.Embedder
	parser   bookparser.Parser
}

func NewProcessor(blob BlobReader, chunks ChunksRepo, embedder embedding.Embedder) (*Processor, error) {
	if blob == nil {
		return nil, fmt.Errorf("nil blob reader")
	}
	if chunks == nil {
		return nil, fmt.Errorf("nil chunks repo")
	}
	if embedder == nil {
		return nil, fmt.Errorf("nil embedder")
	}
	if embedder.Dim() != chunks.Dim() {
		return nil, fmt.Errorf("kb_ingest: embedder dim %d != repo dim %d", embedder.Dim(), chunks.Dim())
	}
	return &Processor{blob: blob, chunks: chunks, embedder: embedder, parser: bookparser.NewTextOnlyParser()}, nil
}

func (p *Processor) Handle(ctx context.Context, lease queue.JobLease) error {
	payload, err := parsePayload(lease.PayloadJSON)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "failed", err.Error())
		return err
	}

	if err := p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "parsing", ""); err != nil {
		return err
	}
	raw, err := p.blob.GetBlob(ctx, payload.WorkspaceRef, payload.BlobSHA256)
	if err != nil {
		return fail(fmt.Errorf("get blob: %w", err))
	}
	doc, err := p.parser.Parse(ctx, bytes.NewReader(raw), payload.MimeType)
	if err != nil {
		return fail(fmt.Errorf("parse: %w", err))
	}

	if err := p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "chunking", ""); err != nil {
		return err
	}
	chunks, err := bookchunker.Chunk(doc, bookchunker.DefaultOptions())
	if err != nil {
		return fail(fmt.Errorf("chunk: %w", err))
	}
	if len(chunks) == 0 {
		if err := p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "ready", ""); err != nil {
			return err
		}
		return nil
	}

	if err := p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "embedding", ""); err != nil {
		return err
	}
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.Text
	}
	vecs, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		return fail(fmt.Errorf("embed: %w", err))
	}
	if len(vecs) != len(chunks) {
		return fail(fmt.Errorf("embed: got %d vectors for %d chunks", len(vecs), len(chunks)))
	}

	if err := p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "upserting", ""); err != nil {
		return err
	}
	rows := make([]data.KBChunkUpsert, len(chunks))
	for i, chunk := range chunks {
		rows[i] = data.KBChunkUpsert{
			KBID:        payload.KBID,
			DocumentID:  payload.DocumentID,
			Ordinal:     chunk.Ordinal,
			HeadingPath: chunk.HeadingPath,
			ChunkType:   chunk.ChunkType,
			Text:        chunk.Text,
			TokenCount:  chunk.TokenCount,
			Embedding:   vecs[i],
		}
	}
	if err := p.chunks.Upsert(ctx, rows); err != nil {
		return fail(fmt.Errorf("upsert: %w", err))
	}
	if err := p.chunks.UpdateDocStatus(ctx, payload.DocumentID, "ready", ""); err != nil {
		return err
	}
	return nil
}

type ingestPayload struct {
	KBID             uuid.UUID
	DocumentID       uuid.UUID
	WorkspaceRef     string
	BlobSHA256       string
	MimeType         string
	OriginalFilename string
}

func parsePayload(raw map[string]any) (ingestPayload, error) {
	kbID, err := parseUUIDField(raw, "kb_id")
	if err != nil {
		return ingestPayload{}, err
	}
	docID, err := parseUUIDField(raw, "document_id")
	if err != nil {
		return ingestPayload{}, err
	}
	p := ingestPayload{
		KBID:             kbID,
		DocumentID:       docID,
		WorkspaceRef:     stringField(raw, "workspace_ref"),
		BlobSHA256:       stringField(raw, "blob_sha256"),
		MimeType:         stringField(raw, "mime_type"),
		OriginalFilename: stringField(raw, "original_filename"),
	}
	if p.WorkspaceRef == "" || p.BlobSHA256 == "" || p.MimeType == "" {
		return ingestPayload{}, fmt.Errorf("kb_ingest payload missing required fields")
	}
	return p, nil
}

func parseUUIDField(raw map[string]any, key string) (uuid.UUID, error) {
	text := stringField(raw, key)
	id, err := uuid.Parse(text)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("kb_ingest payload invalid %s", key)
	}
	return id, nil
}

func stringField(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return strings.TrimSpace(value)
}
