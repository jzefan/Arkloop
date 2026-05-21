package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// KBChunksRepository persists and searches embedded chunks via pgvector.
// M0 uses a single flat table keyed by (kb_name, document_ref, ordinal);
// M1 will replace this with FK-bearing knowledge_bases / kb_documents.
type KBChunksRepository struct {
	pool DB
	dim  int
}

// KBChunkUpsert is the input row for Upsert.
type KBChunkUpsert struct {
	KBName      string
	DocumentRef string
	Ordinal     int
	Text        string
	TokenCount  int
	Embedding   []float32
}

// KBChunkHit is a search result.
type KBChunkHit struct {
	ID          uuid.UUID
	KBName      string
	DocumentRef string
	Ordinal     int
	Text        string
	TokenCount  int
	Score       float32 // cosine similarity in [-1, 1]; 1 = identical
}

// NewKBChunksRepository probes the pgvector column dimension once and caches it.
// This makes Dim cheap and exposes mismatches early (e.g. forgot to migrate).
func NewKBChunksRepository(pool DB) (*KBChunksRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	var dim int
	if err := pool.QueryRow(context.Background(), `
SELECT COALESCE(substring(format_type(a.atttypid, a.atttypmod) from 'vector\(([0-9]+)\)')::int, 0)
FROM   pg_attribute a
JOIN   pg_class c ON c.oid = a.attrelid
JOIN   pg_namespace n ON n.oid = c.relnamespace
WHERE  n.nspname = current_schema()
  AND  c.relname = 'kb_chunks'
  AND  a.attname = 'embedding'
  AND  NOT a.attisdropped`).Scan(&dim); err != nil {
		return nil, fmt.Errorf("probe pgvector dim: %w", err)
	}
	if dim <= 0 {
		return nil, fmt.Errorf("invalid pgvector dim from catalog: %d (run migration 00192?)", dim)
	}
	return &KBChunksRepository{pool: pool, dim: dim}, nil
}

// Dim returns the pgvector column dimension. Always call this before
// constructing Embedder configs to verify they agree.
func (r *KBChunksRepository) Dim() int { return r.dim }

// Upsert writes chunks. Conflict on (kb_name, document_ref, ordinal) is an UPDATE.
func (r *KBChunksRepository) Upsert(ctx context.Context, rows []KBChunkUpsert) error {
	for i, row := range rows {
		if len(row.Embedding) != r.dim {
			return fmt.Errorf("row (kb=%s,doc=%s,ord=%d): embedding dim %d != table dim %d",
				row.KBName, row.DocumentRef, row.Ordinal, len(row.Embedding), r.dim)
		}
		if _, err := r.pool.Exec(ctx, `
INSERT INTO kb_chunks (kb_name, document_ref, ordinal, text, token_count, embedding)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (kb_name, document_ref, ordinal)
DO UPDATE SET text = EXCLUDED.text,
              token_count = EXCLUDED.token_count,
              embedding = EXCLUDED.embedding`,
			row.KBName, row.DocumentRef, row.Ordinal, row.Text, row.TokenCount, vecLiteral(row.Embedding)); err != nil {
			return fmt.Errorf("upsert row %d: %w", i, err)
		}
	}
	return nil
}

// Search returns up to k chunks in kbName ordered by cosine similarity desc.
func (r *KBChunksRepository) Search(ctx context.Context, kbName string, query []float32, k int) ([]KBChunkHit, error) {
	if len(query) != r.dim {
		return nil, fmt.Errorf("query dim %d != table dim %d", len(query), r.dim)
	}
	if k <= 0 {
		k = 8
	}
	rows, err := r.pool.Query(ctx, `
SELECT id, kb_name, document_ref, ordinal, text, token_count,
       (1 - (embedding <=> $2))::real AS score
FROM   kb_chunks
WHERE  kb_name = $1
ORDER  BY embedding <=> $2
LIMIT  $3`,
		kbName, vecLiteral(query), k)
	if err != nil {
		return nil, fmt.Errorf("kb search: %w", err)
	}
	defer rows.Close()

	var out []KBChunkHit
	for rows.Next() {
		var h KBChunkHit
		if err := rows.Scan(&h.ID, &h.KBName, &h.DocumentRef, &h.Ordinal, &h.Text, &h.TokenCount, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// vecLiteral renders a []float32 as pgvector's text representation,
// e.g. "[0.1,0.2,0.3]". pgx encodes this string into the vector column.
func vecLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 6)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}
