package examstore

import "time"

// --- Response types (from exam backend) ---

type KPListResp struct {
	Items []KPItem `json:"items"`
	Total int      `json:"total"`
}

type KPItem struct {
	ID          string  `json:"id"`
	ExamScopeID string  `json:"exam_scope_id"` // renamed from course_id (Q9)
	Code        string  `json:"code"`          // new (Q2): short stable code from exam's curriculum codebook
	DisplayName string  `json:"display_name"`  // renamed from name (Q2): human-readable label
	ParentID    *string `json:"parent_id"`
	Depth       int     `json:"depth"`
	SortOrder   int     `json:"sort_order"`
}

type QListResp struct {
	Items []QItem `json:"items"`
	Total int     `json:"total"`
}

type QItem struct {
	ID               string       `json:"id"`
	KnowledgePointID string       `json:"knowledge_point_id"`
	Type             string       `json:"type"`
	Difficulty       string       `json:"difficulty"`
	Stem             string       `json:"stem"`
	Options          []OptionItem `json:"options"`
	Answer           string       `json:"answer"`
	Explanation      string       `json:"explanation"`
	SourceSnippets   []SnipItem   `json:"source_snippets"`
	PatternTag       string       `json:"pattern_tag"`
	CreatedAt        time.Time    `json:"created_at"`
	CreatedBySource  string       `json:"created_by_source"`
}

type OptionItem struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

type SnipItem struct {
	ChunkRef   string    `json:"chunk_ref"`
	Snippet    string    `json:"snippet"`
	IngestTime time.Time `json:"ingest_time"`
}

// --- Request types (to exam backend) ---

type DraftReq struct {
	KnowledgePointID string       `json:"knowledge_point_id"`
	Type             string       `json:"type"`
	Difficulty       string       `json:"difficulty"`
	Stem             string       `json:"stem"`
	Options          []OptionItem `json:"options,omitempty"`
	Answer           string       `json:"answer"`
	Explanation      string       `json:"explanation,omitempty"`
	SourceSnippets   []SnipItem   `json:"source_snippets,omitempty"`
	PatternTag       string       `json:"pattern_tag,omitempty"`
	CreatedBySource  string       `json:"created_by_source"`
}

type BatchResp struct {
	Created []BatchCreated `json:"created"`
	Failed  []BatchFailed  `json:"failed"`
}

type BatchCreated struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
}

type BatchFailed struct {
	Index        int    `json:"index"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type PaperReq struct {
	Name        string    `json:"name"`
	ExamScopeID string    `json:"exam_scope_id"` // renamed from course_id (Q9)
	Spec        PaperSpec `json:"spec"`
	QuestionIDs []string  `json:"question_ids"`
}

type PaperSpec struct {
	TotalCount                 int            `json:"total_count"`
	TypeDistribution           map[string]int `json:"type_distribution"`
	DifficultyDistribution     map[string]int `json:"difficulty_distribution"`
	KnowledgePointDistribution map[string]int `json:"knowledge_point_distribution"`
	Seed                       int64          `json:"seed"`
}

type PaperResp struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	QuestionCount int       `json:"question_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// ExamScopeListResp is the response body of GET /api/exam-scopes (Q9).
// Scopes form a 3-level hierarchy (major / direction / topic); the
// curriculum tree is reconstructed client-side via parent_id pointers.
type ExamScopeListResp struct {
	Items []ExamScopeItem `json:"items"`
}

type ExamScopeItem struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"` // "major" | "direction" | "topic"
	Code        string  `json:"code"`
	DisplayName string  `json:"display_name"`
	ParentID    *string `json:"parent_id"` // null at major level
}

// ListFilter mirrors the query params for GET /api/questions.
type ListFilter struct {
	Type       string
	Difficulty string
	PatternTag string
	Limit      int
	Offset     int
}
