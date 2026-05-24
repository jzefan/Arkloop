package examstore

import "time"

// --- Response types (from exam backend) ---

type KPListResp struct {
	Items []KPItem `json:"items"`
	Total int      `json:"total"`
}

type KPItem struct {
	ID        string  `json:"id"`
	CourseID  string  `json:"course_id"`
	Name      string  `json:"name"`
	ParentID  *string `json:"parent_id"`
	Depth     int     `json:"depth"`
	SortOrder int     `json:"sort_order"`
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
	Name        string      `json:"name"`
	CourseID    string      `json:"course_id"`
	Spec        PaperSpec   `json:"spec"`
	QuestionIDs []string    `json:"question_ids"`
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

type CourseListResp struct {
	Items []CourseItem `json:"items"`
}

type CourseItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListFilter mirrors the query params for GET /api/questions.
type ListFilter struct {
	Type       string
	Difficulty string
	PatternTag string
	Limit      int
	Offset     int
}
