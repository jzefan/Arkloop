package data

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KBQuestion mirrors a kb_questions row. JSONB columns are stored as raw bytes
// so callers (e.g. questionstore.LocalStore) own marshal/unmarshal of their
// domain types.
type KBQuestion struct {
	ID                 uuid.UUID
	KBID               uuid.UUID
	KnowledgePointID   *uuid.UUID
	QuestionType       string
	Difficulty         string
	Stem               string
	OptionsJSON        []byte
	Answer             string
	Explanation        string
	SourceChunkIDsJSON []byte
	SourceSnippetsJSON []byte
	QualityFlag        string
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// KBQuestionCreate is the input shape for Create.
type KBQuestionCreate struct {
	KBID               uuid.UUID
	KnowledgePointID   *uuid.UUID
	QuestionType       string
	Difficulty         string
	Stem               string
	OptionsJSON        []byte // nil treated as []
	Answer             string
	Explanation        string
	SourceChunkIDsJSON []byte // nil treated as []
	SourceSnippetsJSON []byte // nil treated as []
	QualityFlag        string // "" treated as "draft"
	CreatedBy          *uuid.UUID
}

// KBQuestionFilter is the filter shape for ListByKB.
type KBQuestionFilter struct {
	KnowledgePointID *uuid.UUID
	QuestionType     string // empty -> any
	Difficulty       string // empty -> any
	Limit            int    // <=0 -> no limit
	Offset           int
}

// KBQuestionPatch is the partial-update shape for Update. Pointer fields:
// nil leaves the column unchanged; non-nil overwrites (including empty values).
//
// The knowledge-point FK uses a two-field pattern: set KnowledgePointID to
// non-nil to update to that UUID; set ClearKnowledgePointID true to NULL the
// column. ClearKnowledgePointID is ignored when KnowledgePointID is non-nil.
type KBQuestionPatch struct {
	KnowledgePointID      *uuid.UUID // set to non-nil to update to this UUID
	ClearKnowledgePointID bool       // set true to NULL the FK; ignored if KnowledgePointID is non-nil
	QuestionType          *string
	Difficulty            *string
	Stem                  *string
	OptionsJSON           *[]byte
	Answer                *string
	Explanation           *string
	SourceChunkIDsJSON    *[]byte
	SourceSnippetsJSON    *[]byte
	QualityFlag           *string
}

var (
	// ErrKBQuestionNotFound signals an absent kb_questions row.
	ErrKBQuestionNotFound = errors.New("kb question not found")
)

// KBQuestionsRepository persists kb_questions rows.
type KBQuestionsRepository struct {
	pool DB
}

// NewKBQuestionsRepository constructs the repo.
func NewKBQuestionsRepository(pool DB) (*KBQuestionsRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KBQuestionsRepository{pool: pool}, nil
}

func defaultJSONBytes(b []byte, dflt string) []byte {
	if len(b) == 0 {
		return []byte(dflt)
	}
	return b
}

// Create inserts a new kb_questions row.
func (r *KBQuestionsRepository) Create(ctx context.Context, in KBQuestionCreate) (*KBQuestion, error) {
	quality := in.QualityFlag
	if quality == "" {
		quality = "draft"
	}
	options := defaultJSONBytes(in.OptionsJSON, "[]")
	chunkIDs := defaultJSONBytes(in.SourceChunkIDsJSON, "[]")
	snippets := defaultJSONBytes(in.SourceSnippetsJSON, "[]")

	row := r.pool.QueryRow(ctx, `
INSERT INTO kb_questions (
    kb_id, knowledge_point_id, question_type, difficulty, stem,
    options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
    quality_flag, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, kb_id, knowledge_point_id, question_type, difficulty, stem,
          options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
          quality_flag, created_by, created_at, updated_at`,
		in.KBID, in.KnowledgePointID, in.QuestionType, in.Difficulty, in.Stem,
		options, in.Answer, in.Explanation, chunkIDs, snippets,
		quality, in.CreatedBy)
	return scanKBQuestion(row)
}

// GetByID returns the question or (nil, nil) if absent.
func (r *KBQuestionsRepository) GetByID(ctx context.Context, id uuid.UUID) (*KBQuestion, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, kb_id, knowledge_point_id, question_type, difficulty, stem,
       options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
       quality_flag, created_by, created_at, updated_at
FROM   kb_questions
WHERE  id = $1`, id)
	q, err := scanKBQuestion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return q, err
}

// ListByKB returns matching questions plus the total count (ignoring limit/offset).
func (r *KBQuestionsRepository) ListByKB(ctx context.Context, kbID uuid.UUID, f KBQuestionFilter) ([]KBQuestion, int, error) {
	var (
		whereParts []string
		args       []any
	)
	whereParts = append(whereParts, "kb_id = $1")
	args = append(args, kbID)

	if f.KnowledgePointID != nil {
		args = append(args, *f.KnowledgePointID)
		whereParts = append(whereParts, fmt.Sprintf("knowledge_point_id = $%d", len(args)))
	}
	if f.QuestionType != "" {
		args = append(args, f.QuestionType)
		whereParts = append(whereParts, fmt.Sprintf("question_type = $%d", len(args)))
	}
	if f.Difficulty != "" {
		args = append(args, f.Difficulty)
		whereParts = append(whereParts, fmt.Sprintf("difficulty = $%d", len(args)))
	}
	where := strings.Join(whereParts, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM kb_questions WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count kb_questions: %w", err)
	}

	q := `
SELECT id, kb_id, knowledge_point_id, question_type, difficulty, stem,
       options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
       quality_flag, created_by, created_at, updated_at
FROM   kb_questions
WHERE  ` + where + `
ORDER  BY created_at DESC, id ASC`

	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if f.Offset > 0 {
		args = append(args, f.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []KBQuestion
	for rows.Next() {
		item, err := scanKBQuestionFromRows(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListByKnowledgePoints returns questions whose knowledge_point_id is in the
// supplied set, scoped to a KB. Type/Difficulty/Limit/Offset from the filter
// apply; KnowledgePointID inside the filter is ignored (this method always
// uses the supplied id slice). An empty kpIDs slice returns (nil, nil).
//
// Ordering: by knowledge_point_id ASC, then created_at DESC for a stable read
// useful to paper-composer code that buckets by KP.
func (r *KBQuestionsRepository) ListByKnowledgePoints(ctx context.Context, kbID uuid.UUID, kpIDs []uuid.UUID, f KBQuestionFilter) ([]KBQuestion, error) {
	if len(kpIDs) == 0 {
		return nil, nil
	}
	args := []any{kbID, kpIDs}
	whereParts := []string{"kb_id = $1", "knowledge_point_id = ANY($2)"}
	if f.QuestionType != "" {
		args = append(args, f.QuestionType)
		whereParts = append(whereParts, fmt.Sprintf("question_type = $%d", len(args)))
	}
	if f.Difficulty != "" {
		args = append(args, f.Difficulty)
		whereParts = append(whereParts, fmt.Sprintf("difficulty = $%d", len(args)))
	}
	where := strings.Join(whereParts, " AND ")

	q := `
SELECT id, kb_id, knowledge_point_id, question_type, difficulty, stem,
       options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
       quality_flag, created_by, created_at, updated_at
FROM   kb_questions
WHERE  ` + where + `
ORDER  BY knowledge_point_id ASC, created_at DESC, id ASC`

	if f.Limit > 0 {
		args = append(args, f.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if f.Offset > 0 {
		args = append(args, f.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []KBQuestion
	for rows.Next() {
		item, err := scanKBQuestionFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Update applies a partial patch and returns the updated row. Updated_at is
// bumped whenever at least one field changes.
func (r *KBQuestionsRepository) Update(ctx context.Context, id uuid.UUID, patch KBQuestionPatch) (*KBQuestion, error) {
	var (
		sets []string
		args []any
	)

	switch {
	case patch.KnowledgePointID != nil:
		args = append(args, *patch.KnowledgePointID)
		sets = append(sets, fmt.Sprintf("knowledge_point_id = $%d", len(args)))
	case patch.ClearKnowledgePointID:
		sets = append(sets, "knowledge_point_id = NULL")
	}
	if patch.QuestionType != nil {
		args = append(args, *patch.QuestionType)
		sets = append(sets, fmt.Sprintf("question_type = $%d", len(args)))
	}
	if patch.Difficulty != nil {
		args = append(args, *patch.Difficulty)
		sets = append(sets, fmt.Sprintf("difficulty = $%d", len(args)))
	}
	if patch.Stem != nil {
		args = append(args, *patch.Stem)
		sets = append(sets, fmt.Sprintf("stem = $%d", len(args)))
	}
	if patch.OptionsJSON != nil {
		args = append(args, defaultJSONBytes(*patch.OptionsJSON, "[]"))
		sets = append(sets, fmt.Sprintf("options_json = $%d", len(args)))
	}
	if patch.Answer != nil {
		args = append(args, *patch.Answer)
		sets = append(sets, fmt.Sprintf("answer = $%d", len(args)))
	}
	if patch.Explanation != nil {
		args = append(args, *patch.Explanation)
		sets = append(sets, fmt.Sprintf("explanation = $%d", len(args)))
	}
	if patch.SourceChunkIDsJSON != nil {
		args = append(args, defaultJSONBytes(*patch.SourceChunkIDsJSON, "[]"))
		sets = append(sets, fmt.Sprintf("source_chunk_ids_json = $%d", len(args)))
	}
	if patch.SourceSnippetsJSON != nil {
		args = append(args, defaultJSONBytes(*patch.SourceSnippetsJSON, "[]"))
		sets = append(sets, fmt.Sprintf("source_snippets_json = $%d", len(args)))
	}
	if patch.QualityFlag != nil {
		args = append(args, *patch.QualityFlag)
		sets = append(sets, fmt.Sprintf("quality_flag = $%d", len(args)))
	}

	if len(sets) == 0 {
		// No-op patch: just return current row, preserving not-found semantics
		// so callers see ErrKBQuestionNotFound consistently with the SET path.
		out, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, ErrKBQuestionNotFound
		}
		return out, nil
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf(`
UPDATE kb_questions
SET    %s
WHERE  id = $%d
RETURNING id, kb_id, knowledge_point_id, question_type, difficulty, stem,
          options_json, answer, explanation, source_chunk_ids_json, source_snippets_json,
          quality_flag, created_by, created_at, updated_at`,
		strings.Join(sets, ", "), len(args))

	row := r.pool.QueryRow(ctx, q, args...)
	out, err := scanKBQuestion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrKBQuestionNotFound
	}
	return out, err
}

// Delete removes a question.
func (r *KBQuestionsRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM kb_questions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKBQuestionNotFound
	}
	return nil
}

// CountByKB returns the total question count in a KB.
func (r *KBQuestionsRepository) CountByKB(ctx context.Context, kbID uuid.UUID) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM kb_questions WHERE kb_id = $1`, kbID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count by kb: %w", err)
	}
	return n, nil
}

func scanKBQuestion(row pgx.Row) (*KBQuestion, error) {
	var q KBQuestion
	if err := row.Scan(
		&q.ID, &q.KBID, &q.KnowledgePointID, &q.QuestionType, &q.Difficulty, &q.Stem,
		&q.OptionsJSON, &q.Answer, &q.Explanation, &q.SourceChunkIDsJSON, &q.SourceSnippetsJSON,
		&q.QualityFlag, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt); err != nil {
		return nil, err
	}
	return &q, nil
}

func scanKBQuestionFromRows(rows pgx.Rows) (*KBQuestion, error) {
	var q KBQuestion
	if err := rows.Scan(
		&q.ID, &q.KBID, &q.KnowledgePointID, &q.QuestionType, &q.Difficulty, &q.Stem,
		&q.OptionsJSON, &q.Answer, &q.Explanation, &q.SourceChunkIDsJSON, &q.SourceSnippetsJSON,
		&q.QualityFlag, &q.CreatedBy, &q.CreatedAt, &q.UpdatedAt); err != nil {
		return nil, err
	}
	return &q, nil
}
