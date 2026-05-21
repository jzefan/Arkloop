// Package kbingest composes bookchunker + embedding + data.KBChunksRepository
// into the M0 Ingester/Searcher used by the kb-debug HTTP handlers. M1 will
// move this composition into a worker job.
package kbingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/http/kbdebugapi"
	"arkloop/services/shared/bookchunker"
	"arkloop/services/shared/embedding"
)

// Service implements kbdebugapi.Ingester and kbdebugapi.Searcher.
type Service struct {
	embedder embedding.Embedder
	repo     *data.KBChunksRepository
}

// New constructs a Service. The embedder's Dim() must match repo.Dim().
func New(embedder embedding.Embedder, repo *data.KBChunksRepository) (*Service, error) {
	if embedder == nil {
		return nil, fmt.Errorf("nil embedder")
	}
	if repo == nil {
		return nil, fmt.Errorf("nil kb chunks repo")
	}
	if embedder.Dim() != repo.Dim() {
		return nil, fmt.Errorf("kbingest: embedder dim %d != repo dim %d (S0 mismatch?)",
			embedder.Dim(), repo.Dim())
	}
	return &Service{embedder: embedder, repo: repo}, nil
}

// Ingest reads filePath, chunks it, embeds, and upserts under kbName.
// document_ref is the file basename.
func (s *Service) Ingest(ctx context.Context, filePath, kbName string) (int, error) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}
	chunks, err := bookchunker.Chunk(string(buf), bookchunker.DefaultOptions())
	if err != nil {
		return 0, fmt.Errorf("chunk: %w", err)
	}
	if len(chunks) == 0 {
		return 0, nil
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	vecs, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}
	if len(vecs) != len(chunks) {
		return 0, fmt.Errorf("embed: got %d vectors for %d chunks", len(vecs), len(chunks))
	}

	docRef := filepath.Base(filePath)
	rows := make([]data.KBChunkUpsert, len(chunks))
	for i, c := range chunks {
		rows[i] = data.KBChunkUpsert{
			KBName:      kbName,
			DocumentRef: docRef,
			Ordinal:     c.Ordinal,
			Text:        c.Text,
			TokenCount:  c.TokenCount,
			Embedding:   vecs[i],
		}
	}
	if err := s.repo.Upsert(ctx, rows); err != nil {
		return 0, fmt.Errorf("upsert: %w", err)
	}
	return len(chunks), nil
}

// Search embeds the query and retrieves the nearest chunks from pgvector.
func (s *Service) Search(ctx context.Context, kbName, query string, k int) ([]kbdebugapi.SearchHit, error) {
	vecs, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embed query: got %d vectors, want 1", len(vecs))
	}
	hits, err := s.repo.Search(ctx, kbName, vecs[0], k)
	if err != nil {
		return nil, fmt.Errorf("repo search: %w", err)
	}
	out := make([]kbdebugapi.SearchHit, len(hits))
	for i, h := range hits {
		out[i] = kbdebugapi.SearchHit{
			DocumentRef: h.DocumentRef,
			Ordinal:     h.Ordinal,
			Text:        h.Text,
			Score:       h.Score,
		}
	}
	return out, nil
}
