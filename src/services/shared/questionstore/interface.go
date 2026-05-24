package questionstore

import "context"

// QuestionStore abstracts "where questions/papers/knowledge-points live".
// Linked mode → examstore (HTTP to exam backend).
// Standalone mode → localstore (direct Postgres).
type QuestionStore interface {
	ListReferenceQuestions(ctx context.Context, knowledgePointID string, filter ListFilter) ([]Question, int, error)
	SaveQuestions(ctx context.Context, drafts []QuestionDraft) (SaveResult, error)
	ListKnowledgePoints(ctx context.Context, courseOrKBID string) ([]KnowledgePoint, error)
	SavePaper(ctx context.Context, name string, courseOrKBID string, spec PaperSpec, questionIDs []string) (string, error)
	ListQuestionsForPaperPool(ctx context.Context, knowledgePointIDs []string, filter ListFilter) ([]Question, error)
}
