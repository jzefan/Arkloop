package markdowntopdf

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "markdown_to_pdf"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "convert Markdown content to a formal PDF artifact",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr("Convert Markdown report content into a downloadable formal A4 PDF artifact. Use after final report Markdown is complete."),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type": "string",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "PDF filename, e.g. 深圳职业技术大学_产教融合指数报告_2026.pdf",
			},
			"template": map[string]any{
				"type":    "string",
				"enum":    []string{"formal_report"},
				"default": "formal_report",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "final Markdown content",
			},
		},
		"required":             []string{"filename", "content"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
