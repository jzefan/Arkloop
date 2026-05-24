package exam

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const (
	ToolNameListSeedQuestions = "exam_list_seed_questions"
	ToolNameBuildQuestions    = "exam_build_questions"
	ToolNameSaveQuestions     = "exam_save_questions"
	ToolNameBuildPaper        = "exam_build_paper"
)

var ListSeedQuestionsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameListSeedQuestions,
	Version:     "1",
	Description: "list existing exam questions as seed/reference samples for item building",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var BuildQuestionsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameBuildQuestions,
	Version:     "1",
	Description: "generate new questions guided by a skill and seed questions (does NOT save to exam)",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var SaveQuestionsAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameSaveQuestions,
	Version:     "1",
	Description: "save teacher-confirmed questions to exam (supports partial failure)",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

var BuildPaperAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameBuildPaper,
	Version:     "1",
	Description: "compose a paper from exam question pool and save to exam",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true,
}

var ListSeedQuestionsLlmSpec = llm.ToolSpec{
	Name: ToolNameListSeedQuestions,
	Description: strPtr(
		"列出 exam 中指定知识点下的已有题目作为种子/参考样本。支持按 type、difficulty、pattern_tag 过滤。",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"knowledge_point_id": map[string]any{"type": "string", "description": "exam 知识点 UUID"},
			"type":               map[string]any{"type": "string", "description": "题型过滤（single_choice 等）"},
			"difficulty":         map[string]any{"type": "string", "enum": []string{"easy", "medium", "hard"}},
			"pattern_tag":        map[string]any{"type": "string", "description": "命题模式标签（A1/A2/A3/A4 等）"},
			"limit":              map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 10},
		},
		"required":             []string{"knowledge_point_id"},
		"additionalProperties": false,
	},
}

var BuildQuestionsLlmSpec = llm.ToolSpec{
	Name: ToolNameBuildQuestions,
	Description: strPtr(
		"按命题技能风格，参考种子题生成新题。不写库——返给 persona 让老师预览确认。" +
			"pattern_tag 强校验：输出必须与种子题一致，不一致的题标记 pattern_tag_mismatch。",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"seed_question_ids": map[string]any{
				"type": "array", "items": map[string]any{"type": "string"},
				"description": "种子题 ID 列表（从 exam_list_seed_questions 选出）",
			},
			"skill_key": map[string]any{"type": "string", "description": "命题技能 skill_key"},
			"count":     map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "default": 5},
		},
		"required":             []string{"seed_question_ids", "skill_key"},
		"additionalProperties": false,
	},
}

var SaveQuestionsLlmSpec = llm.ToolSpec{
	Name: ToolNameSaveQuestions,
	Description: strPtr(
		"把老师确认的题目批量写入 exam。支持部分失败：成功的返回 exam ID，失败的返回 error_code + error_message。" +
			"传入 expected_pattern_tag 时启用 PRD O2-C 强校验：每道题 pattern_tag 必须等于该值，否则进 Failed (error_code=pattern_tag_mismatch)。",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expected_pattern_tag": map[string]any{
				"type":        "string",
				"description": "可选，启用 pattern_tag 强校验。来自 exam_build_questions 返回的 expected_pattern_tag。",
			},
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
						"pattern_tag":        map[string]any{"type": "string"},
					},
					"required": []string{"knowledge_point_id", "type", "stem", "answer"},
				},
			},
		},
		"required":             []string{"questions"},
		"additionalProperties": false,
	},
}

var BuildPaperLlmSpec = llm.ToolSpec{
	Name: ToolNameBuildPaper,
	Description: strPtr(
		"从 exam 题池组卷并写回 exam。先拉题池 → 按约束抽样 → 题不够时返回 shortage_warnings → 老师确认后保存。",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"exam_scope_id":           map[string]any{"type": "string", "description": "exam 范围 UUID（专业/方向/主知识点任一级，对应 knowledge_bases.exam_scope_id）"},
			"name":                    map[string]any{"type": "string", "description": "试卷名称"},
			"knowledge_point_ids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"total_count":             map[string]any{"type": "integer", "minimum": 1},
			"type_distribution":       map[string]any{"type": "object", "description": "{type→count}"},
			"difficulty_distribution": map[string]any{"type": "object", "description": "{level→count}"},
			"seed":                    map[string]any{"type": "integer", "description": "随机种子（可选，用于复现）"},
		},
		"required":             []string{"exam_scope_id", "name", "knowledge_point_ids", "total_count"},
		"additionalProperties": false,
	},
}
