// Package kbingest contains the M0 debug ingester compatibility shell.
// M1.0 moves KB ingestion into an async worker job under the real KB schema.
package kbingest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/http/kbdebugapi"
	"arkloop/services/shared/bookchunker"
	"arkloop/services/shared/bookparser"
	"arkloop/services/shared/embedding"
)

// Service implements kbdebugapi.Ingester and kbdebugapi.Searcher until the
// M0 debug routes are removed.
type Service struct {
	embedder embedding.Embedder
	repo     *data.KBChunksRepository
	parser   bookparser.Parser
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
		return nil, fmt.Errorf("kbingest: embedder dim %d != repo dim %d", embedder.Dim(), repo.Dim())
	}
	return &Service{embedder: embedder, repo: repo, parser: bookparser.NewTextOnlyParser()}, nil
}

// Ingest parses/chunks input to keep the M1 chunker integration compiled,
// then refuses the M0 flat-schema operation. Task 8 deletes the debug route.
func (s *Service) Ingest(ctx context.Context, filePath, kbName string) (int, error) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}
	doc, err := s.parser.Parse(ctx, bytes.NewReader(buf), guessMimeFromExt(filePath))
	if err != nil {
		return 0, fmt.Errorf("parse: %w", err)
	}
	chunks, err := bookchunker.Chunk(doc, bookchunker.DefaultOptions())
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
	if _, err := s.embedder.Embed(ctx, texts); err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}

	_ = kbName
	return 0, fmt.Errorf("kbingest.Ingest is M0-only and incompatible with the M1.0 kb_chunks schema; use the KB REST endpoints instead")
}

// Search embeds nothing and refuses the M0 flat-schema operation. Task 8
// deletes the debug route.
func (s *Service) Search(ctx context.Context, kbName, query string, k int) ([]kbdebugapi.SearchHit, error) {
	_ = ctx
	_ = kbName
	_ = query
	_ = k
	return nil, fmt.Errorf("kbingest.Search is M0-only and incompatible with the M1.0 kb_chunks schema; use the KB REST endpoints instead")
}

func guessMimeFromExt(p string) string {
	switch filepath.Ext(p) {
	case ".md":
		return "text/markdown"
	default:
		return "text/plain"
	}
}
