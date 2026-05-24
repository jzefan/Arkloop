package kb

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolNameSearch = "kb_search"
const ToolNameExtractTOC = "kb_extract_toc"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolNameSearch,
	Version:     "1",
	Description: "semantic retrieval over a workspace-scoped knowledge base",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var ExtractTOCAgentSpec = tools.AgentToolSpec{
	Name:        ToolNameExtractTOC,
	Version:     "1",
	Description: "extract a document's TOC tree (heading hierarchy) for catalog creation in exam",
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

var ExtractTOCLlmSpec = llm.ToolSpec{
	Name:        ToolNameExtractTOC,
	Description: stringPtr("从已摄入的 KB 文档中提取目录树（heading 层级结构）。返回 tree + node_count；node_count < 5 时 tree 为 null。用于 Linked KB 建目录到 exam。"),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kb_id": map[string]any{
				"type":        "string",
				"description": "KB UUID",
			},
			"document_id": map[string]any{
				"type":        "string",
				"description": "文档 UUID",
			},
		},
		"required":             []string{"kb_id", "document_id"},
		"additionalProperties": false,
	},
}

func stringPtr(v string) *string { return &v }
