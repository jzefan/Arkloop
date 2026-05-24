package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KBPaper mirrors a kb_papers row. JSONB columns are stored as raw bytes so
// the spec/question-id encoding is owned by callers (papers service).
type KBPaper struct {
	ID              uuid.UUID
	KBID            uuid.UUID
	Name            string
	SpecJSON        []byte
	Seed            int64
	QuestionIDsJSON []byte
	Markdown        string
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// KBPaperCreate is the input shape for Create.
type KBPaperCreate struct {
	KBID            uuid.UUID
	Name            string
	SpecJSON        []byte // nil treated as {}
	Seed            int64
	QuestionIDsJSON []byte // nil treated as []
	Markdown        string
	CreatedBy       *uuid.UUID
}

// ErrKBPaperNotFound signals an absent kb_papers row.
var ErrKBPaperNotFound = errors.New("kb paper not found")

// KBPapersRepository persists kb_papers rows.
type KBPapersRepository struct {
	pool DB
}

// NewKBPapersRepository constructs the repo.
func NewKBPapersRepository(pool DB) (*KBPapersRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KBPapersRepository{pool: pool}, nil
}

// Create inserts a new kb_papers row.
func (r *KBPapersRepository) Create(ctx context.Context, in KBPaperCreate) (*KBPaper, error) {
	spec := in.SpecJSON
	if len(spec) == 0 {
		spec = []byte("{}")
	}
	qids := in.QuestionIDsJSON
	if len(qids) == 0 {
		qids = []byte("[]")
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO kb_papers (kb_id, name, spec_json, seed, question_ids_json, markdown, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, kb_id, name, spec_json, seed, question_ids_json, markdown, created_by, created_at, updated_at`,
		in.KBID, in.Name, spec, in.Seed, qids, in.Markdown, in.CreatedBy)
	return scanKBPaper(row)
}

// GetByID returns the paper or (nil, nil) if absent.
func (r *KBPapersRepository) GetByID(ctx context.Context, id uuid.UUID) (*KBPaper, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, kb_id, name, spec_json, seed, question_ids_json, markdown, created_by, created_at, updated_at
FROM   kb_papers
WHERE  id = $1`, id)
	p, err := scanKBPaper(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

// ListByKB returns all papers in a KB, newest first.
func (r *KBPapersRepository) ListByKB(ctx context.Context, kbID uuid.UUID) ([]KBPaper, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, kb_id, name, spec_json, seed, question_ids_json, markdown, created_by, created_at, updated_at
FROM   kb_papers
WHERE  kb_id = $1
ORDER  BY created_at DESC, id ASC`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KBPaper
	for rows.Next() {
		p, err := scanKBPaperFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Delete removes a paper.
func (r *KBPapersRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM kb_papers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKBPaperNotFound
	}
	return nil
}

func scanKBPaper(row pgx.Row) (*KBPaper, error) {
	var p KBPaper
	if err := row.Scan(&p.ID, &p.KBID, &p.Name, &p.SpecJSON, &p.Seed, &p.QuestionIDsJSON,
		&p.Markdown, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanKBPaperFromRows(rows pgx.Rows) (*KBPaper, error) {
	var p KBPaper
	if err := rows.Scan(&p.ID, &p.KBID, &p.Name, &p.SpecJSON, &p.Seed, &p.QuestionIDsJSON,
		&p.Markdown, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
