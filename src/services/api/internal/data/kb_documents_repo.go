package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type KBDocument struct {
	ID               uuid.UUID
	KBID             uuid.UUID
	OriginalFilename string
	MimeType         string
	BlobSHA256       string
	SizeBytes        int64
	Status           string
	ErrorMessage     string
	ParseMeta        map[string]any
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type DocCreate struct {
	KBID             uuid.UUID
	OriginalFilename string
	MimeType         string
	BlobSHA256       string
	SizeBytes        int64
	CreatedBy        *uuid.UUID
}

var ErrDocNotFound = errors.New("kb document not found")

type KBDocumentsRepository struct {
	pool DB
}

func NewKBDocumentsRepository(pool DB) (*KBDocumentsRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KBDocumentsRepository{pool: pool}, nil
}

func (r *KBDocumentsRepository) Create(ctx context.Context, in DocCreate) (*KBDocument, error) {
	row := r.pool.QueryRow(ctx, `
INSERT INTO kb_documents (kb_id, original_filename, mime_type, blob_sha256, size_bytes, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at`,
		in.KBID, in.OriginalFilename, in.MimeType, in.BlobSHA256, in.SizeBytes, in.CreatedBy)
	return scanDoc(row)
}

func (r *KBDocumentsRepository) GetByID(ctx context.Context, id uuid.UUID) (*KBDocument, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at
FROM   kb_documents
WHERE  id = $1`, id)
	doc, err := scanDoc(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return doc, err
}

func (r *KBDocumentsRepository) ListByKB(ctx context.Context, kbID uuid.UUID) ([]KBDocument, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at
FROM   kb_documents
WHERE  kb_id = $1
ORDER  BY created_at DESC`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KBDocument
	for rows.Next() {
		doc, err := scanDocFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *doc)
	}
	return out, rows.Err()
}

// UpdateStatus moves a doc through the state machine and optionally records error_message + parse_meta.
// parseMeta is nil-safe; pass nil to leave parse_meta_json unchanged.
func (r *KBDocumentsRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMessage string, parseMeta map[string]any) error {
	if parseMeta != nil {
		buf, err := json.Marshal(parseMeta)
		if err != nil {
			return err
		}
		tag, err := r.pool.Exec(ctx, `
UPDATE kb_documents
SET    status = $2, error_message = $3, parse_meta_json = $4, updated_at = now()
WHERE  id = $1`, id, status, errorMessage, buf)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrDocNotFound
		}
		return nil
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE kb_documents
SET    status = $2, error_message = $3, updated_at = now()
WHERE  id = $1`, id, status, errorMessage)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNotFound
	}
	return nil
}

func (r *KBDocumentsRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM kb_documents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNotFound
	}
	return nil
}

func scanDoc(row pgx.Row) (*KBDocument, error) {
	var d KBDocument
	var metaRaw []byte
	err := row.Scan(&d.ID, &d.KBID, &d.OriginalFilename, &d.MimeType, &d.BlobSHA256, &d.SizeBytes,
		&d.Status, &d.ErrorMessage, &metaRaw, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &d.ParseMeta)
	}
	if d.ParseMeta == nil {
		d.ParseMeta = map[string]any{}
	}
	return &d, nil
}

func scanDocFromRows(rows pgx.Rows) (*KBDocument, error) {
	var d KBDocument
	var metaRaw []byte
	err := rows.Scan(&d.ID, &d.KBID, &d.OriginalFilename, &d.MimeType, &d.BlobSHA256, &d.SizeBytes,
		&d.Status, &d.ErrorMessage, &metaRaw, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &d.ParseMeta)
	}
	if d.ParseMeta == nil {
		d.ParseMeta = map[string]any{}
	}
	return &d, nil
}
