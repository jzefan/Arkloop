// Package localstore implements questionstore.QuestionStore against the
// Standalone-mode kb_knowledge_points / kb_questions / kb_papers tables.
//
// One Store instance is bound to a single KB at construction time; the KB id
// passed to interface methods (courseOrKBID / knowledgePointID owner) is
// expected to match — when it disagrees, the method-supplied id wins, on the
// principle that the caller's intent is more current than the constructor's
// initial scope.
//
// PatternTag is silently dropped on save and never populated on read: the
// Standalone tier has no pattern tagging, which lives only in the exam tier
// (Option 2). CreatedBySource is likewise unsupported — kb_questions has no
// column for "ai" / "human" provenance distinct from quality_flag.
package localstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"arkloop/services/api/internal/data"
	"arkloop/services/shared/questionstore"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Listing and paper-pool limits. ListReferenceQuestions is meant for
// human-facing reference picking; the paper-pool path feeds the composer.
const (
	defaultListLimit     = 10
	minListLimit         = 1
	maxListLimit         = 50
	defaultPoolListLimit = 200
	maxPoolListLimit     = 500
)

// Store is the Standalone QuestionStore. The kbID set at construction is the
// default tenant scope; method args carrying a courseOrKBID override on a
// per-call basis (defensive — should always equal kbID in normal callers).
type Store struct {
	deps  Dependencies
	kbID  uuid.UUID
	valid bool // false when the constructor was passed a malformed kbID
}

// New builds a Store. A malformed kbID is preserved so every method returns
// ErrInvalidKBID; we don't panic here because the factory wires us from a
// string and we'd prefer a runtime error over a boot panic.
func New(deps Dependencies, kbID string) *Store {
	parsed, err := uuid.Parse(kbID)
	if err != nil {
		return &Store{deps: deps, valid: false}
	}
	return &Store{deps: deps, kbID: parsed, valid: true}
}

// resolveKB chooses between the constructor-scoped kbID and the method-passed
// courseOrKBID. The method param wins when present, mirroring the contract.
func (s *Store) resolveKB(courseOrKBID string) (uuid.UUID, error) {
	if courseOrKBID != "" {
		parsed, err := uuid.Parse(courseOrKBID)
		if err != nil {
			return uuid.Nil, ErrInvalidKBID
		}
		return parsed, nil
	}
	if !s.valid {
		return uuid.Nil, ErrInvalidKBID
	}
	return s.kbID, nil
}

func clamp(v, lo, hi, dflt int) int {
	if v <= 0 {
		return dflt
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ListReferenceQuestions returns reference questions scoped to a single
// knowledge point in the Store's KB. The PatternTag filter field is ignored
// here — Standalone has no pattern tags.
func (s *Store) ListReferenceQuestions(ctx context.Context, knowledgePointID string, filter questionstore.ListFilter) ([]questionstore.Question, int, error) {
	if !s.valid {
		return nil, 0, ErrInvalidKBID
	}
	kpID, err := uuid.Parse(knowledgePointID)
	if err != nil {
		return nil, 0, ErrInvalidKnowledgePointID
	}

	limit := clamp(filter.Limit, minListLimit, maxListLimit, defaultListLimit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, total, err := s.deps.Questions.ListByKB(ctx, s.kbID, data.KBQuestionFilter{
		KnowledgePointID: &kpID,
		QuestionType:     filter.Type,
		Difficulty:       filter.Difficulty,
		Limit:            limit,
		Offset:           offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list reference questions: %w", err)
	}
	out := make([]questionstore.Question, 0, len(rows))
	for _, r := range rows {
		out = append(out, toQuestionstoreQuestion(r))
	}
	return out, total, nil
}

// SaveQuestions inserts drafts one by one, never aborting the batch on a
// single failure. Validation issues and DB errors are reported per-draft via
// SaveResult.Failed with stable error codes.
func (s *Store) SaveQuestions(ctx context.Context, drafts []questionstore.QuestionDraft) (questionstore.SaveResult, error) {
	if !s.valid {
		return questionstore.SaveResult{}, ErrInvalidKBID
	}
	result := questionstore.SaveResult{}
	for i, d := range drafts {
		if msg, ok := validateDraft(d); !ok {
			result.Failed = append(result.Failed, questionstore.SaveFailure{
				Index:        i,
				ErrorCode:    "validation_error",
				ErrorMessage: msg,
			})
			continue
		}

		var kpID *uuid.UUID
		if d.KnowledgePointID != "" {
			parsed, err := uuid.Parse(d.KnowledgePointID)
			if err != nil {
				result.Failed = append(result.Failed, questionstore.SaveFailure{
					Index:        i,
					ErrorCode:    "knowledge_point_id_invalid",
					ErrorMessage: "knowledge_point_id is not a valid UUID",
				})
				continue
			}
			kpID = &parsed
		}

		optionsJSON, err := marshalOrEmpty(d.Options, "[]")
		if err != nil {
			result.Failed = append(result.Failed, questionstore.SaveFailure{
				Index:        i,
				ErrorCode:    "validation_error",
				ErrorMessage: "options not serialisable",
			})
			continue
		}
		snippetsJSON, err := marshalOrEmpty(d.SourceSnippets, "[]")
		if err != nil {
			result.Failed = append(result.Failed, questionstore.SaveFailure{
				Index:        i,
				ErrorCode:    "validation_error",
				ErrorMessage: "source_snippets not serialisable",
			})
			continue
		}

		// PatternTag and CreatedBySource: no kb_questions column for either.
		// Standalone has no pattern tagging; provenance lives in quality_flag
		// only with different semantics. Drop both silently.
		created, err := s.deps.Questions.Create(ctx, data.KBQuestionCreate{
			KBID:               s.kbID,
			KnowledgePointID:   kpID,
			QuestionType:       d.Type,
			Difficulty:         d.Difficulty,
			Stem:               d.Stem,
			OptionsJSON:        optionsJSON,
			Answer:             d.Answer,
			Explanation:        d.Explanation,
			SourceSnippetsJSON: snippetsJSON,
		})
		if err != nil {
			code, msg := classifySaveError(err)
			result.Failed = append(result.Failed, questionstore.SaveFailure{
				Index:        i,
				ErrorCode:    code,
				ErrorMessage: msg,
			})
			continue
		}
		result.Created = append(result.Created, questionstore.SavedQuestion{
			Index: i,
			ID:    created.ID.String(),
		})
	}
	return result, nil
}

// ListKnowledgePoints returns all KPs for the resolved KB. Depth is reported
// as 0 because kb_knowledge_points does not persist tree depth; walking the
// parent chain on every call is too expensive for the read path and not
// required by the M2a Standalone contract.
func (s *Store) ListKnowledgePoints(ctx context.Context, courseOrKBID string) ([]questionstore.KnowledgePoint, error) {
	kbID, err := s.resolveKB(courseOrKBID)
	if err != nil {
		return nil, err
	}
	rows, err := s.deps.KnowledgePoints.ListByKB(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("list knowledge points: %w", err)
	}
	out := make([]questionstore.KnowledgePoint, 0, len(rows))
	for _, r := range rows {
		var parent *string
		if r.ParentID != nil {
			s := r.ParentID.String()
			parent = &s
		}
		out = append(out, questionstore.KnowledgePoint{
			ID:        r.ID.String(),
			Name:      r.Name,
			ParentID:  parent,
			Depth:     0,
			SortOrder: r.SortOrder,
		})
	}
	return out, nil
}

// SavePaper persists structural paper data (spec, seed, ordered question
// id list). Markdown rendering is performed by a separate downstream path
// (paper export) and is not the store's concern.
func (s *Store) SavePaper(ctx context.Context, name string, courseOrKBID string, spec questionstore.PaperSpec, questionIDs []string) (string, error) {
	kbID, err := s.resolveKB(courseOrKBID)
	if err != nil {
		return "", err
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal paper spec: %w", err)
	}
	// Preserve ordering and duplicates exactly as supplied — composition
	// callers rely on indices matching the spec's distribution layout.
	idsJSON, err := json.Marshal(questionIDs)
	if err != nil {
		return "", fmt.Errorf("marshal question ids: %w", err)
	}

	created, err := s.deps.Papers.Create(ctx, data.KBPaperCreate{
		KBID:            kbID,
		Name:            name,
		SpecJSON:        specJSON,
		Seed:            spec.Seed,
		QuestionIDsJSON: idsJSON,
		Markdown:        "",
	})
	if err != nil {
		return "", fmt.Errorf("create paper: %w", err)
	}
	return created.ID.String(), nil
}

// ListQuestionsForPaperPool aggregates questions across multiple KPs for the
// paper composer. Limit/offset are clamped to a higher ceiling than the
// reference list path because the composer needs a sizeable pool.
func (s *Store) ListQuestionsForPaperPool(ctx context.Context, knowledgePointIDs []string, filter questionstore.ListFilter) ([]questionstore.Question, error) {
	if !s.valid {
		return nil, ErrInvalidKBID
	}
	kpIDs := make([]uuid.UUID, 0, len(knowledgePointIDs))
	for _, raw := range knowledgePointIDs {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, ErrInvalidKnowledgePointID
		}
		kpIDs = append(kpIDs, parsed)
	}

	limit := clamp(filter.Limit, minListLimit, maxPoolListLimit, defaultPoolListLimit)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.deps.Questions.ListByKnowledgePoints(ctx, s.kbID, kpIDs, data.KBQuestionFilter{
		QuestionType: filter.Type,
		Difficulty:   filter.Difficulty,
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list questions for paper pool: %w", err)
	}
	out := make([]questionstore.Question, 0, len(rows))
	for _, r := range rows {
		out = append(out, toQuestionstoreQuestion(r))
	}
	return out, nil
}

// --- helpers ---

// toQuestionstoreQuestion maps a data.KBQuestion to the public type. JSONB
// columns that fail to unmarshal are logged and reduced to nil slices —
// returning an error on a single mangled row would punish every other caller
// of the listing endpoint.
func toQuestionstoreQuestion(r data.KBQuestion) questionstore.Question {
	q := questionstore.Question{
		ID:          r.ID.String(),
		Type:        r.QuestionType,
		Difficulty:  r.Difficulty,
		Stem:        r.Stem,
		Answer:      r.Answer,
		Explanation: r.Explanation,
		CreatedAt:   r.CreatedAt,
	}
	if r.KnowledgePointID != nil {
		q.KnowledgePointID = r.KnowledgePointID.String()
	}
	if len(r.OptionsJSON) > 0 {
		var opts []questionstore.QuestionOption
		if err := json.Unmarshal(r.OptionsJSON, &opts); err != nil {
			log.Printf("localstore: options_json unmarshal failed for question %s: %v", r.ID, err)
		} else {
			q.Options = opts
		}
	}
	if len(r.SourceSnippetsJSON) > 0 {
		var snips []questionstore.SourceSnippet
		if err := json.Unmarshal(r.SourceSnippetsJSON, &snips); err != nil {
			log.Printf("localstore: source_snippets_json unmarshal failed for question %s: %v", r.ID, err)
		} else {
			q.SourceSnippets = snips
		}
	}
	// PatternTag and CreatedBySource intentionally left zero — see package doc.
	return q
}

func marshalOrEmpty(v any, empty string) ([]byte, error) {
	// json.Marshal on a nil slice yields "null"; the kb_questions repo
	// substitutes "[]" when the byte slice is empty, so an explicit empty
	// slice and a nil slice land in the same place.
	switch x := v.(type) {
	case []questionstore.QuestionOption:
		if len(x) == 0 {
			return []byte(empty), nil
		}
	case []questionstore.SourceSnippet:
		if len(x) == 0 {
			return []byte(empty), nil
		}
	}
	return json.Marshal(v)
}

// validateDraft mirrors the kb_questions CHECK constraints client-side so we
// can give callers a structured error code instead of leaking a raw Postgres
// message.
func validateDraft(d questionstore.QuestionDraft) (string, bool) {
	if d.Stem == "" {
		return "stem must not be empty", false
	}
	if !isValidQuestionType(d.Type) {
		return fmt.Sprintf("invalid question type: %q", d.Type), false
	}
	if !isValidDifficulty(d.Difficulty) {
		return fmt.Sprintf("invalid difficulty: %q", d.Difficulty), false
	}
	return "", true
}

func isValidQuestionType(t string) bool {
	switch t {
	case "single_choice", "multi_choice", "fill_in", "short_answer", "essay":
		return true
	}
	return false
}

func isValidDifficulty(d string) bool {
	switch d {
	case "easy", "medium", "hard":
		return true
	}
	return false
}

// classifySaveError maps a DB-layer error to a (code, public_message) pair.
// The raw DB message is logged but never returned, to avoid leaking schema
// detail to callers.
func classifySaveError(err error) (string, string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503": // foreign_key_violation
			// Only kb_id and knowledge_point_id are FKs on kb_questions; kb_id
			// comes from the Store's bound scope and is presumed valid, so any
			// FK violation here points at knowledge_point_id.
			log.Printf("localstore: FK violation on save: %s constraint=%s", pgErr.Code, pgErr.ConstraintName)
			return "knowledge_point_not_found", "referenced knowledge_point does not exist"
		case "23514": // check_violation
			log.Printf("localstore: check violation on save: %s constraint=%s", pgErr.Code, pgErr.ConstraintName)
			return "validation_error", "value violates database constraint"
		}
		log.Printf("localstore: unclassified pg error %s: %s", pgErr.Code, pgErr.Message)
	} else {
		log.Printf("localstore: save error: %v", err)
	}
	return "internal_error", "internal error while saving question"
}
