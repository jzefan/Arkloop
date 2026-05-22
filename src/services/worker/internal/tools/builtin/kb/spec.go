package kb

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolNameSearch = "kb_search"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolNameSearch,
	Version:     "1",
	Description: "semantic retrieval over a workspace-scoped knowledge base",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolNameSearch,
	Description: stringPtr("在指定知识库（KB）中按语义相似度检索教材片段。返回 document_ref、段落号、章节路径、原文与相似度。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kb_id": map[string]any{
				"type":        "string",
				"description": "KB UUID。",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "中文或英文检索词、问题。",
			},
			"k": map[string]any{
				"type":        "integer",
				"description": "返回 chunk 数，默认 8，最大 50。",
				"default":     8,
				"minimum":     1,
				"maximum":     50,
			},
		},
		"required":             []string{"kb_id", "query"},
		"additionalProperties": false,
	},
}

func stringPtr(v string) *string { return &v }
