package questionstore

import "time"

// Question represents a question from either exam or local store.
type Question struct {
	ID               string
	KnowledgePointID string
	Type             string // single_choice, multi_choice, fill_in, short_answer, essay
	Difficulty       string // easy, medium, hard
	Stem             string
	Options          []QuestionOption
	Answer           string
	Explanation      string
	SourceSnippets   []SourceSnippet
	PatternTag       string // A1, A2, A3, A4, etc.
	CreatedBySource  string // ai, human
	CreatedAt        time.Time
}

type QuestionOption struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type SourceSnippet struct {
	ChunkRef   string    `json:"chunk_ref"`
	Snippet    string    `json:"snippet"`
	IngestTime time.Time `json:"ingest_time"`
}

// QuestionDraft is the input for saving new questions.
type QuestionDraft struct {
	KnowledgePointID string
	Type             string
	Difficulty       string
	Stem             string
	Options          []QuestionOption
	Answer           string
	Explanation      string
	SourceSnippets   []SourceSnippet
	PatternTag       string
	CreatedBySource  string
}

type SavedQuestion struct {
	Index int
	ID    string
}

type SaveFailure struct {
	Index        int
	ErrorCode    string
	ErrorMessage string
}

type SaveResult struct {
	Created []SavedQuestion
	Failed  []SaveFailure
}

type KnowledgePoint struct {
	ID        string
	Name      string
	ParentID  *string
	Depth     int
	SortOrder int
}

type PaperSpec struct {
	TotalCount                 int
	TypeDistribution           map[string]int
	DifficultyDistribution     map[string]int
	KnowledgePointDistribution map[string]int
	Seed                       int64
}

type ListFilter struct {
	Type       string
	Difficulty string
	PatternTag string
	Limit      int
	Offset     int
}

// KBDescriptor carries the minimal KB info needed by the factory.
type KBDescriptor struct {
	ID              string
	IntegrationMode string // "standalone" | "exam"
	ExamCourseID    string
}
