package examstore

import (
	"context"
	"fmt"

	"arkloop/services/shared/questionstore"
)

// TokenSource provides the teacher's OIDC token for each request.
type TokenSource func(ctx context.Context) (string, error)

// Store adapts Client to the questionstore.QuestionStore interface.
type Store struct {
	client      *Client
	tokenSource TokenSource
	examScopeID string
}

func NewStore(client *Client, tokenSource TokenSource, examScopeID string) *Store {
	return &Store{client: client, tokenSource: tokenSource, examScopeID: examScopeID}
}

func (s *Store) token(ctx context.Context) (string, error) {
	tok, err := s.tokenSource(ctx)
	if err != nil {
		return "", fmt.Errorf("examstore: token: %w", err)
	}
	return tok, nil
}

func (s *Store) ListReferenceQuestions(ctx context.Context, kpID string, filter questionstore.ListFilter) ([]questionstore.Question, int, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return nil, 0, err
	}
	resp, err := s.client.ListQuestions(ctx, tok, kpID, ListFilter{
		Type: filter.Type, Difficulty: filter.Difficulty, PatternTag: filter.PatternTag,
		Limit: filter.Limit, Offset: filter.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]questionstore.Question, len(resp.Items))
	for i, q := range resp.Items {
		out[i] = mapQuestion(q)
	}
	return out, resp.Total, nil
}

func (s *Store) SaveQuestions(ctx context.Context, drafts []questionstore.QuestionDraft) (questionstore.SaveResult, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return questionstore.SaveResult{}, err
	}
	reqs := make([]DraftReq, len(drafts))
	for i, d := range drafts {
		reqs[i] = mapDraft(d)
	}
	resp, err := s.client.CreateQuestionsBatch(ctx, tok, reqs)
	if err != nil {
		return questionstore.SaveResult{}, err
	}
	var result questionstore.SaveResult
	for _, c := range resp.Created {
		result.Created = append(result.Created, questionstore.SavedQuestion{Index: c.Index, ID: c.ID})
	}
	for _, f := range resp.Failed {
		result.Failed = append(result.Failed, questionstore.SaveFailure{
			Index: f.Index, ErrorCode: f.ErrorCode, ErrorMessage: f.ErrorMessage,
		})
	}
	return result, nil
}

func (s *Store) ListKnowledgePoints(ctx context.Context, _ string) ([]questionstore.KnowledgePoint, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.ListKnowledgePoints(ctx, tok, s.examScopeID, 500, 0)
	if err != nil {
		return nil, err
	}
	out := make([]questionstore.KnowledgePoint, len(resp.Items))
	for i, kp := range resp.Items {
		out[i] = questionstore.KnowledgePoint{
			ID: kp.ID, Code: kp.Code, Name: kp.DisplayName, ParentID: kp.ParentID,
			Depth: kp.Depth, SortOrder: kp.SortOrder,
		}
	}
	return out, nil
}

func (s *Store) SavePaper(ctx context.Context, name string, _ string, spec questionstore.PaperSpec, questionIDs []string) (string, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return "", err
	}
	resp, err := s.client.CreatePaper(ctx, tok, PaperReq{
		Name:        name,
		ExamScopeID: s.examScopeID,
		Spec: PaperSpec{
			TotalCount:                 spec.TotalCount,
			TypeDistribution:           spec.TypeDistribution,
			DifficultyDistribution:     spec.DifficultyDistribution,
			KnowledgePointDistribution: spec.KnowledgePointDistribution,
			Seed:                       spec.Seed,
		},
		QuestionIDs: questionIDs,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (s *Store) ListQuestionsForPaperPool(ctx context.Context, kpIDs []string, filter questionstore.ListFilter) ([]questionstore.Question, error) {
	tok, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	// Aggregate across knowledge points (exam API takes one kpID at a time)
	var all []questionstore.Question
	for _, kpID := range kpIDs {
		resp, err := s.client.ListQuestions(ctx, tok, kpID, ListFilter{
			Type: filter.Type, Difficulty: filter.Difficulty, PatternTag: filter.PatternTag,
			Limit: 200,
		})
		if err != nil {
			return nil, fmt.Errorf("list questions for kp %s: %w", kpID, err)
		}
		for _, q := range resp.Items {
			all = append(all, mapQuestion(q))
		}
	}
	return all, nil
}

// --- mapping helpers ---

func mapQuestion(q QItem) questionstore.Question {
	opts := make([]questionstore.QuestionOption, len(q.Options))
	for i, o := range q.Options {
		opts[i] = questionstore.QuestionOption{Key: o.Key, Text: o.Text}
	}
	snips := make([]questionstore.SourceSnippet, len(q.SourceSnippets))
	for i, s := range q.SourceSnippets {
		snips[i] = questionstore.SourceSnippet{ChunkRef: s.ChunkRef, Snippet: s.Snippet, IngestTime: s.IngestTime}
	}
	return questionstore.Question{
		ID: q.ID, KnowledgePointID: q.KnowledgePointID,
		Type: q.Type, Difficulty: q.Difficulty, Stem: q.Stem,
		Options: opts, Answer: q.Answer, Explanation: q.Explanation,
		SourceSnippets: snips, PatternTag: q.PatternTag,
		CreatedBySource: q.CreatedBySource, CreatedAt: q.CreatedAt,
	}
}

func mapDraft(d questionstore.QuestionDraft) DraftReq {
	opts := make([]OptionItem, len(d.Options))
	for i, o := range d.Options {
		opts[i] = OptionItem{Key: o.Key, Text: o.Text}
	}
	snips := make([]SnipItem, len(d.SourceSnippets))
	for i, s := range d.SourceSnippets {
		snips[i] = SnipItem{ChunkRef: s.ChunkRef, Snippet: s.Snippet, IngestTime: s.IngestTime}
	}
	return DraftReq{
		KnowledgePointID: d.KnowledgePointID, Type: d.Type, Difficulty: d.Difficulty,
		Stem: d.Stem, Options: opts, Answer: d.Answer, Explanation: d.Explanation,
		SourceSnippets: snips, PatternTag: d.PatternTag, CreatedBySource: d.CreatedBySource,
	}
}

// Register wires examstore into the questionstore factory.
func init() {
	questionstore.NewExamStoreFunc = func(examScopeID string) questionstore.QuestionStore {
		// This will be called by the factory; the actual Client + TokenSource
		// must be set via SetGlobalClient before any KB with mode=exam is used.
		if globalClient == nil {
			return nil
		}
		return NewStore(globalClient, globalTokenSource, examScopeID)
	}
}

var (
	globalClient      *Client
	globalTokenSource TokenSource
)

// SetGlobalClient configures the package-level client used by the factory.
// Called once at api/worker startup when ARKLOOP_EXAM_INTEGRATION_ENABLED=true.
func SetGlobalClient(client *Client, tokenSource TokenSource) {
	globalClient = client
	globalTokenSource = tokenSource
}
