package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KnowledgeBase mirrors a knowledge_bases row.
type KnowledgeBase struct {
	ID              uuid.UUID
	WorkspaceRef    string
	AccountID       uuid.UUID
	Name            string
	Description     string
	Visibility      string // 'workspace_member' | 'private'
	IntegrationMode string
	ExamScopeID     *string
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DocumentCount   int
}

// KBCreate is the input shape for Create.
type KBCreate struct {
	AccountID       uuid.UUID
	WorkspaceRef    string
	Name            string
	Description     string
	Visibility      string // "" treated as "workspace_member"
	IntegrationMode string // "" treated as "standalone"
	ExamScopeID     *string
	CreatedBy       *uuid.UUID
}

var (
	// ErrKBNotFound signals a non-existent or already-deleted KB.
	ErrKBNotFound = errors.New("knowledge base not found")
	// ErrKBDuplicateName signals a UNIQUE constraint violation on (workspace_ref, name).
	ErrKBDuplicateName = errors.New("knowledge base name already exists in this workspace")
)

// KnowledgeBasesRepository persists knowledge_bases rows.
type KnowledgeBasesRepository struct {
	pool DB
}

// NewKnowledgeBasesRepository constructs the repo.
func NewKnowledgeBasesRepository(pool DB) (*KnowledgeBasesRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KnowledgeBasesRepository{pool: pool}, nil
}

// Create inserts a new knowledge_bases row.
func (r *KnowledgeBasesRepository) Create(ctx context.Context, in KBCreate) (*KnowledgeBase, error) {
	visibility := in.Visibility
	if visibility == "" {
		visibility = "workspace_member"
	}
	mode := in.IntegrationMode
	if mode == "" {
		mode = "standalone"
	}
	row := r.pool.QueryRow(ctx, `
INSERT INTO knowledge_bases (workspace_ref, account_id, name, description, visibility, integration_mode, exam_scope_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, workspace_ref, account_id, name, description, visibility, integration_mode, exam_scope_id, created_by, created_at, updated_at`,
		in.WorkspaceRef, in.AccountID, in.Name, in.Description, visibility, mode, in.ExamScopeID, in.CreatedBy)
	var kb KnowledgeBase
	if err := row.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
		&kb.Visibility, &kb.IntegrationMode, &kb.ExamScopeID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
		if isPGUniqueViolation(err) {
			return nil, ErrKBDuplicateName
		}
		return nil, fmt.Errorf("create kb: %w", err)
	}
	return &kb, nil
}

// GetByID returns the KB or (nil, nil) if absent.
func (r *KnowledgeBasesRepository) GetByID(ctx context.Context, id uuid.UUID) (*KnowledgeBase, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, workspace_ref, account_id, name, description, visibility, integration_mode, exam_scope_id, created_by, created_at, updated_at
FROM   knowledge_bases
WHERE  id = $1`, id)
	var kb KnowledgeBase
	if err := row.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
		&kb.Visibility, &kb.IntegrationMode, &kb.ExamScopeID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &kb, nil
}

// ListByWorkspace returns all KBs in (account_id, workspace_ref), with DocumentCount populated.
func (r *KnowledgeBasesRepository) ListByWorkspace(ctx context.Context, accountID uuid.UUID, workspaceRef string) ([]KnowledgeBase, error) {
	rows, err := r.pool.Query(ctx, `
SELECT kb.id, kb.workspace_ref, kb.account_id, kb.name, kb.description,
       kb.visibility, kb.integration_mode, kb.exam_scope_id, kb.created_by, kb.created_at, kb.updated_at,
       COALESCE((SELECT COUNT(*) FROM kb_documents d WHERE d.kb_id = kb.id), 0) AS document_count
FROM   knowledge_bases kb
WHERE  kb.account_id = $1 AND kb.workspace_ref = $2
ORDER  BY kb.created_at DESC`, accountID, workspaceRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KnowledgeBase
	for rows.Next() {
		var kb KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
			&kb.Visibility, &kb.IntegrationMode, &kb.ExamScopeID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt, &kb.DocumentCount); err != nil {
			return nil, err
		}
		out = append(out, kb)
	}
	return out, rows.Err()
}

// Delete removes a KB; chunks and documents cascade.
func (r *KnowledgeBasesRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM knowledge_bases WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKBNotFound
	}
	return nil
}

func isPGUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
