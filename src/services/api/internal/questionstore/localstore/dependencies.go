package localstore

import (
	"arkloop/services/api/internal/data"
	"arkloop/services/shared/questionstore"
)

// Dependencies bundles the api repos that back the Standalone-mode
// QuestionStore implementation. The repos are passed as concrete pointers
// because all live in the same api module; there is no need for a layer of
// adapter interfaces.
type Dependencies struct {
	KnowledgeBases  *data.KnowledgeBasesRepository
	KnowledgePoints *data.KBKnowledgePointsRepository
	Questions       *data.KBQuestionsRepository
	Papers          *data.KBPapersRepository
}

// Register stores the dependencies and wires the questionstore factory
// injection point so questionstore.For(kb, ...) can produce a Store without
// pulling api/internal/data into the shared module (an internal-package
// import that would not compile).
//
// Must be called once during application boot, before any QuestionStore.For()
// call from worker tools or HTTP handlers.
func Register(d Dependencies) {
	questionstore.NewLocalStoreFunc = func(kbID string) questionstore.QuestionStore {
		return New(d, kbID)
	}
}
