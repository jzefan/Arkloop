package markdowntopdf

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "markdown_to_pdf"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "2",
	Description: "convert Markdown content to a formatted A4 PDF artifact (headings, lists, tables, images, code blocks). Uses a system CJK font.",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name: ToolName,
	Description: strPtr(
		"Convert Markdown report content into a downloadable A4 PDF artifact. " +
			"Supports headings (H1-H6), paragraphs, ordered/unordered lists (with nesting), " +
			"GitHub-flavoured tables, images (http/https/data URIs), fenced code blocks, " +
			"blockquotes, horizontal rules and inline links. " +
			"The default font is the host system's CJK TrueType font (e.g. Songti on macOS). " +
			"Pass font_path to override with a specific .ttf/.ttc file. " +
			"Use after final report Markdown is complete.",
	),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Shown in the page header and used as the H1 if the markdown does not start with one.",
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
			"font_path": map[string]any{
				"type":        "string",
				"description": "Optional absolute path to a TrueType font (.ttf) or TrueType Collection (.ttc). When unset, the tool probes standard OS font paths.",
			},
			"image_roots": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional whitelist of directories from which local image references may be loaded. When empty, only remote (http/https) and data URI images are accepted.",
			},
		},
		"required":             []string{"filename", "content"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
