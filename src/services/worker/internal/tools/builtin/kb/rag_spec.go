package kb

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const (
	ToolNameListKnowledgeBases  = "kb_list_knowledge_bases"
	ToolNameListKnowledgePoints = "kb_list_knowledge_points"
	ToolNameDraftQuestions      = "kb_draft_questions"
	ToolNameSaveQuestions       = "kb_save_questions"
	ToolNameComposePaper        = "kb_compose_paper"
)

var ListKnowledgeBasesAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameListKnowledgeBases,
	Version:     "1",
	Description: "list teacher-available ready knowledge bases for paper building",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var ListKnowledgePointsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameListKnowledgePoints,
	Version:     "1",
	Description: "list knowledge points for a KB (standalone: local table; linked: exam backend)",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var DraftQuestionsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameDraftQuestions,
	Version:     "1",
	Description: "generate question drafts using KB content + reference questions (does NOT save)",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var SaveQuestionsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameSaveQuestions,
	Version:     "1",
	Description: "save teacher-confirmed generated questions to the account-level paper-building question bank",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

var ComposePaperAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameComposePaper,
	Version:     "1",
	Description: "compose a paper from the paper-building question bank; saves only when confirmed=true",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

var ListKnowledgePointsLlmSpec = llm.ToolSpec{
	Name:        ToolNameListKnowledgePoints,
	Description: stringPtr("列出指定 KB 的知识点。Standalone 模式返回本地知识点；Linked 模式返回 exam 知识点树。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kb_id": map[string]any{"type": "string", "description": "KB UUID"},
		},
		"required":             []string{"kb_id"},
		"additionalProperties": false,
	},
}

var ListKnowledgeBasesLlmSpec = llm.ToolSpec{
	Name:        ToolNameListKnowledgeBases,
	Description: stringPtr("列出当前老师可用且已有 ready 文档的课程资料知识库。用于让老师选择课程资料，不需要老师手填 KB UUID。默认不返回系统固定的组卷题库。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workspace_ref": map[string]any{
				"type":        "string",
				"description": "可选工作区 ref；省略时列当前 Account 下可见 KB，不按当前聊天临时 workspace 过滤。",
			},
			"ready_only": map[string]any{
				"type":        "boolean",
				"description": "是否只返回至少有一个 ready 文档的 KB，默认 true。",
				"default":     true,
			},
			"include_system_banks": map[string]any{
				"type":        "boolean",
				"description": "是否包含系统固定题库（如组卷题库），默认 false。",
				"default":     false,
			},
		},
		"additionalProperties": false,
	},
}

var DraftQuestionsLlmSpec = llm.ToolSpec{
	Name:        ToolNameDraftQuestions,
	Description: stringPtr("基于 KB 教材内容 + 已有题参考，生成题目草稿（≤5 道）。不写库，返给老师预览。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kb_id":              map[string]any{"type": "string", "description": "KB UUID"},
			"knowledge_point_id": map[string]any{"type": "string", "description": "知识点 ID"},
			"count":              map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "default": 5},
			"type":               map[string]any{"type": "string", "description": "题型（single_choice/fill_in 等）"},
			"difficulty":         map[string]any{"type": "string", "enum": []string{"easy", "medium", "hard"}},
			"retrieval_query":    map[string]any{"type": "string", "description": "用于 kb_search 的检索词（默认用知识点名）"},
		},
		"required":             []string{"kb_id", "knowledge_point_id"},
		"additionalProperties": false,
	},
}

var SaveQuestionsLlmSpec = llm.ToolSpec{
	Name:        ToolNameSaveQuestions,
	Description: stringPtr("把老师确认的题目保存到组卷题库。必须在老师明确确认后传 confirmed=true；否则工具拒绝保存。支持部分失败。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirmed": map[string]any{
				"type":        "boolean",
				"description": "老师明确确认保存后必须为 true。",
				"const":       true,
			},
			"kb_id": map[string]any{"type": "string", "description": "KB UUID"},
			"questions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"knowledge_point_id": map[string]any{"type": "string"},
						"type":               map[string]any{"type": "string"},
						"difficulty":         map[string]any{"type": "string"},
						"stem":               map[string]any{"type": "string"},
						"options":            map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
						"answer":             map[string]any{"type": "string"},
						"explanation":        map[string]any{"type": "string"},
					},
					"required": []string{"knowledge_point_id", "type", "stem", "answer"},
				},
			},
		},
		"required":             []string{"confirmed", "kb_id", "questions"},
		"additionalProperties": false,
	},
}

var ComposePaperLlmSpec = llm.ToolSpec{
	Name:        ToolNameComposePaper,
	Description: stringPtr("从组卷题库题池组卷。未传 confirmed=true 时只返回预览；老师确认后再传 confirmed=true 保存试卷。题不够时返回 shortage_warnings。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirmed":               map[string]any{"type": "boolean", "description": "老师确认保存试卷后传 true；省略或 false 只预览。"},
			"kb_id":                   map[string]any{"type": "string", "description": "KB UUID"},
			"name":                    map[string]any{"type": "string", "description": "试卷名称"},
			"knowledge_point_ids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"total_count":             map[string]any{"type": "integer", "minimum": 1},
			"type_distribution":       map[string]any{"type": "object", "description": "{type→count}"},
			"difficulty_distribution": map[string]any{"type": "object", "description": "{level→count}"},
			"knowledge_point_distribution": map[string]any{
				"type":        "object",
				"description": "{knowledge_point_id→count}；用于指定各知识点题量。",
			},
			"seed": map[string]any{"type": "integer", "description": "随机种子（可选）"},
		},
		"required":             []string{"kb_id", "name", "knowledge_point_ids", "total_count"},
		"additionalProperties": false,
	},
}
