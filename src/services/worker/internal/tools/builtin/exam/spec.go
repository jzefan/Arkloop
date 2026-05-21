package exam

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

// ─── Tool names ───────────────────────────────────────────────────────

const (
	ToolNameRecognizeCatalogImage = "exam_recognize_catalog_image"
	ToolNameParseCatalogExcel     = "exam_parse_catalog_excel"
	ToolNameCreateCatalogTree     = "exam_create_catalog_tree"
	ToolNameGenerateQuestions     = "exam_generate_questions"
)

// ─── AgentSpecs (worker → policy registration) ────────────────────────

var RecognizeCatalogImageAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameRecognizeCatalogImage,
	Version:     "1",
	Description: "recognize a course catalog from uploaded textbook table-of-contents images (returns a structured tree without writing to DB)",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var ParseCatalogExcelAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameParseCatalogExcel,
	Version:     "1",
	Description: "parse a 3-column Excel file (course name | chapter name | section name) into a structured catalog tree",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var CreateCatalogTreeAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameCreateCatalogTree,
	Version:     "1",
	Description: "persist a previously-recognized catalog tree into exam: creates the direction + chapters + sections",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

var GenerateQuestionsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameGenerateQuestions,
	Version:     "1",
	Description: "generate AI questions for the given knowledge point IDs (call repeatedly with small batches for incremental display)",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

// ─── LlmSpecs (JSON schema exposed to the model) ──────────────────────

var RecognizeCatalogImageLlmSpec = llm.ToolSpec{
	Name: ToolNameRecognizeCatalogImage,
	Description: strPtr(
		"Recognize a course catalog from one or more textbook table-of-contents images. " +
			"Returns a structured tree (course → chapters → sections) without writing to DB. " +
			"Use this BEFORE exam_create_catalog_tree so the user can review the recognition result.",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"image_urls": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "List of http(s) or data: URLs of catalog images uploaded by the user",
			},
		},
		"required":             []string{"image_urls"},
		"additionalProperties": false,
	},
}

var ParseCatalogExcelLlmSpec = llm.ToolSpec{
	Name: ToolNameParseCatalogExcel,
	Description: strPtr(
		"Parse an .xlsx file with three columns: 课程名 | 章名 | 节名. " +
			"Returns the same tree shape as exam_recognize_catalog_image so callers can use either source uniformly.",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the .xlsx file (typically inside the user's workspace)",
			},
			"sheet": map[string]any{
				"type":        "string",
				"description": "Sheet name to read; defaults to the first sheet if omitted",
			},
		},
		"required":             []string{"file_path"},
		"additionalProperties": false,
	},
}

var CreateCatalogTreeLlmSpec = llm.ToolSpec{
	Name: ToolNameCreateCatalogTree,
	Description: strPtr(
		"Persist a catalog tree into exam. Creates a learning Direction for the course, " +
			"a top-level knowledge point per chapter, and child knowledge points per section. " +
			"Idempotent on (course_name) — re-running with the same name will reuse the existing direction.",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"course_name": map[string]any{
				"type":        "string",
				"description": "Top-level course / direction name (e.g. 高等数学)",
			},
			"chapters": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string"},
						"sections": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
					"required":             []string{"name", "sections"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"course_name", "chapters"},
		"additionalProperties": false,
	},
}

var GenerateQuestionsLlmSpec = llm.ToolSpec{
	Name: ToolNameGenerateQuestions,
	Description: strPtr(
		"Generate AI-authored questions for the given knowledge point IDs. " +
			"Returns the freshly generated question objects. " +
			"Call repeatedly with small batches (count<=5) to give the user incremental progress.",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"knowledge_point_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "UUIDs of knowledge points to anchor questions to",
			},
			"count": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"maximum":     5,
				"description": "Number of questions to generate this call. Keep small for responsive UX.",
			},
			"difficulty": map[string]any{
				"type":        "string",
				"enum":        []string{"easy", "medium", "hard"},
				"default":     "medium",
				"description": "Target difficulty level",
			},
			"type_distribution": map[string]any{
				"type":        "object",
				"description": "Mapping of question type → desired count (e.g. {single_choice: 3, fill_in: 2}); must sum to `count`.",
			},
		},
		"required":             []string{"knowledge_point_ids", "count"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
