package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KBKnowledgePoint mirrors a kb_knowledge_points row. In Standalone-mode KBs the
// name is a free-form label; in Linked (exam) mode rows mirror exam knowledge
// points and carry the upstream id in ExamKnowledgePointID.
type KBKnowledgePoint struct {
	ID                   uuid.UUID
	KBID                 uuid.UUID
	Name                 string
	ParentID             *uuid.UUID
	ExamKnowledgePointID *string
	SortOrder            int
	CreatedAt            time.Time
}

// KBKnowledgePointCreate is the input shape for Create.
type KBKnowledgePointCreate struct {
	KBID                 uuid.UUID
	Name                 string
	ParentID             *uuid.UUID
	ExamKnowledgePointID *string
	SortOrder            int
}

// ErrKBKnowledgePointNotFound signals an absent kb_knowledge_points row.
var ErrKBKnowledgePointNotFound = errors.New("kb knowledge point not found")

// KBKnowledgePointsRepository persists kb_knowledge_points and the
// kb_document_knowledge_points association table.
type KBKnowledgePointsRepository struct {
	pool DB
}

// NewKBKnowledgePointsRepository constructs the repo.
func NewKBKnowledgePointsRepository(pool DB) (*KBKnowledgePointsRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KBKnowledgePointsRepository{pool: pool}, nil
}

// Create inserts a new kb_knowledge_points row.
func (r *KBKnowledgePointsRepository) Create(ctx context.Context, in KBKnowledgePointCreate) (*KBKnowledgePoint, error) {
	row := r.pool.QueryRow(ctx, `
INSERT INTO kb_knowledge_points (kb_id, name, parent_id, exam_knowledge_point_id, sort_order)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, kb_id, name, parent_id, exam_knowledge_point_id, sort_order, created_at`,
		in.KBID, in.Name, in.ParentID, in.ExamKnowledgePointID, in.SortOrder)
	var kp KBKnowledgePoint
	if err := row.Scan(&kp.ID, &kp.KBID, &kp.Name, &kp.ParentID, &kp.ExamKnowledgePointID, &kp.SortOrder, &kp.CreatedAt); err != nil {
		return nil, fmt.Errorf("create knowledge point: %w", err)
	}
	return &kp, nil
}

// GetByID returns the knowledge point or (nil, nil) if absent.
func (r *KBKnowledgePointsRepository) GetByID(ctx context.Context, id uuid.UUID) (*KBKnowledgePoint, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, kb_id, name, parent_id, exam_knowledge_point_id, sort_order, created_at
FROM   kb_knowledge_points
WHERE  id = $1`, id)
	var kp KBKnowledgePoint
	if err := row.Scan(&kp.ID, &kp.KBID, &kp.Name, &kp.ParentID, &kp.ExamKnowledgePointID, &kp.SortOrder, &kp.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &kp, nil
}

// ListByKB returns all knowledge points in a KB, ordered by (sort_order, created_at).
func (r *KBKnowledgePointsRepository) ListByKB(ctx context.Context, kbID uuid.UUID) ([]KBKnowledgePoint, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, kb_id, name, parent_id, exam_knowledge_point_id, sort_order, created_at
FROM   kb_knowledge_points
WHERE  kb_id = $1
ORDER  BY sort_order ASC, created_at ASC`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KBKnowledgePoint
	for rows.Next() {
		var kp KBKnowledgePoint
		if err := rows.Scan(&kp.ID, &kp.KBID, &kp.Name, &kp.ParentID, &kp.ExamKnowledgePointID, &kp.SortOrder, &kp.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, kp)
	}
	return out, rows.Err()
}

// Delete removes a knowledge point. Associations cascade.
func (r *KBKnowledgePointsRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM kb_knowledge_points WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKBKnowledgePointNotFound
	}
	return nil
}

// AssociateDocument links a document to a knowledge point. Idempotent.
func (r *KBKnowledgePointsRepository) AssociateDocument(ctx context.Context, kbID, docID, kpID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO kb_document_knowledge_points (kb_id, document_id, knowledge_point_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`, kbID, docID, kpID)
	if err != nil {
		return fmt.Errorf("associate document: %w", err)
	}
	return nil
}

// ListByDocument returns all knowledge points associated with a document,
// ordered by sort_order (then created_at) for stable output.
func (r *KBKnowledgePointsRepository) ListByDocument(ctx context.Context, docID uuid.UUID) ([]KBKnowledgePoint, error) {
	rows, err := r.pool.Query(ctx, `
SELECT kp.id, kp.kb_id, kp.name, kp.parent_id, kp.exam_knowledge_point_id, kp.sort_order, kp.created_at
FROM   kb_knowledge_points kp
JOIN   kb_document_knowledge_points m ON m.knowledge_point_id = kp.id
WHERE  m.document_id = $1
ORDER  BY kp.sort_order ASC, kp.created_at ASC`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KBKnowledgePoint
	for rows.Next() {
		var kp KBKnowledgePoint
		if err := rows.Scan(&kp.ID, &kp.KBID, &kp.Name, &kp.ParentID, &kp.ExamKnowledgePointID, &kp.SortOrder, &kp.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, kp)
	}
	return out, rows.Err()
}
