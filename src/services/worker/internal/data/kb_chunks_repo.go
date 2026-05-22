package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KBChunksRepository struct {
	pool *pgxpool.Pool
	dim  int
}

type KBChunkUpsert struct {
	KBID        uuid.UUID
	DocumentID  uuid.UUID
	Ordinal     int
	HeadingPath []string
	ChunkType   string
	Text        string
	TokenCount  int
	Embedding   []float32
}

type KBChunkHit struct {
	ID          uuid.UUID
	KBID        uuid.UUID
	DocumentID  uuid.UUID
	DocumentRef string
	Ordinal     int
	HeadingPath []string
	ChunkType   string
	Text        string
	TokenCount  int
	Score       float32
}

func NewKBChunksRepository(pool *pgxpool.Pool) (*KBChunksRepository, error) {
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
		return nil, fmt.Errorf("invalid pgvector dim from catalog: %d (run migration 00193?)", dim)
	}
	return &KBChunksRepository{pool: pool, dim: dim}, nil
}

func (r *KBChunksRepository) Dim() int { return r.dim }

func (r *KBChunksRepository) Upsert(ctx context.Context, rows []KBChunkUpsert) error {
	if len(rows) == 0 {
		return nil
	}
	for i, row := range rows {
		if len(row.Embedding) != r.dim {
			return fmt.Errorf("row %d: dim %d != table %d", i, len(row.Embedding), r.dim)
		}
	}
	for _, row := range rows {
		headingPath := row.HeadingPath
		if headingPath == nil {
			headingPath = []string{}
		}
		chunkType := row.ChunkType
		if chunkType == "" {
			chunkType = "paragraph"
		}
		_, err := r.pool.Exec(ctx, `
INSERT INTO kb_chunks (kb_id, document_id, ordinal, heading_path, chunk_type, text, token_count, embedding, metadata_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '{}'::jsonb)
ON CONFLICT (kb_id, document_id, ordinal) DO UPDATE SET
    heading_path = EXCLUDED.heading_path,
    chunk_type   = EXCLUDED.chunk_type,
    text         = EXCLUDED.text,
    token_count  = EXCLUDED.token_count,
    embedding    = EXCLUDED.embedding`,
			row.KBID, row.DocumentID, row.Ordinal, headingPath, chunkType, row.Text, row.TokenCount, vecLiteral(row.Embedding))
		if err != nil {
			return fmt.Errorf("upsert kb=%s doc=%s ord=%d: %w", row.KBID, row.DocumentID, row.Ordinal, err)
		}
	}
	return nil
}

func (r *KBChunksRepository) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]KBChunkHit, error) {
	if len(q) != r.dim {
		return nil, fmt.Errorf("query dim %d != table dim %d", len(q), r.dim)
	}
	if k <= 0 {
		k = 8
	}
	rows, err := r.pool.Query(ctx, `
SELECT c.id, c.kb_id, c.document_id, d.original_filename, c.ordinal, c.heading_path, c.chunk_type, c.text, c.token_count,
       (1 - (c.embedding <=> $2))::real AS score
FROM   kb_chunks c
JOIN   kb_documents d ON d.id = c.document_id
WHERE  c.kb_id = $1
ORDER  BY c.embedding <=> $2
LIMIT  $3`, kbID, vecLiteral(q), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KBChunkHit
	for rows.Next() {
		var h KBChunkHit
		if err := rows.Scan(&h.ID, &h.KBID, &h.DocumentID, &h.DocumentRef, &h.Ordinal,
			&h.HeadingPath, &h.ChunkType, &h.Text, &h.TokenCount, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *KBChunksRepository) UpdateDocStatus(ctx context.Context, docID uuid.UUID, status, errorMessage string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE kb_documents
SET    status = $2, error_message = $3, updated_at = now()
WHERE  id = $1`, docID, status, errorMessage)
	return err
}

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
