//go:build desktop

package lsp

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

// positionOp builds a oneOf branch for operations requiring file_path + line + column.
func positionOp(op, desc string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{"type": "string", "const": op},
			"file_path": map[string]any{"type": "string", "description": "Absolute or workspace-relative path to the file."},
			"line":      map[string]any{"type": "integer", "description": "1-based line number."},
			"column":    map[string]any{"type": "integer", "description": "1-based column number (byte offset in line)."},
		},
		"required":             []string{"operation", "file_path", "line", "column"},
		"additionalProperties": false,
		"description":          desc,
	}
}

var LSPToolLLMSpec = llm.ToolSpec{
	Name: "lsp",
	Description: strPtr("Code intelligence via LSP. Operations: definition, references, hover, " +
		"document_symbols, workspace_symbols, type_definition, implementation, diagnostics, rename, " +
		"prepare_call_hierarchy, incoming_calls, outgoing_calls."),
	JSONSchema: map[string]any{
		"oneOf": []map[string]any{
			positionOp("definition", "Go to definition of symbol at position."),
			{
				"type": "object",
				"properties": map[string]any{
					"operation":           map[string]any{"type": "string", "const": "references"},
					"file_path":           map[string]any{"type": "string", "description": "Absolute or workspace-relative path to the file."},
					"line":                map[string]any{"type": "integer", "description": "1-based line number."},
					"column":              map[string]any{"type": "integer", "description": "1-based column number (byte offset in line)."},
					"include_declaration": map[string]any{"type": "boolean", "description": "Include declaration in results. Default true."},
				},
				"required":             []string{"operation", "file_path", "line", "column"},
				"additionalProperties": false,
				"description":          "Find all references to symbol at position.",
			},
			positionOp("hover", "Get hover info (type, docs) for symbol at position."),
			{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{"type": "string", "const": "document_symbols"},
					"file_path": map[string]any{"type": "string", "description": "Absolute or workspace-relative path to the file."},
				},
				"required":             []string{"operation", "file_path"},
				"additionalProperties": false,
				"description":          "List all symbols in a document.",
			},
			{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{"type": "string", "const": "workspace_symbols"},
					"query":     map[string]any{"type": "string", "description": "Search query for symbols."},
				},
				"required":             []string{"operation", "query"},
				"additionalProperties": false,
				"description":          "Search symbols across the workspace.",
			},
			positionOp("type_definition", "Go to type definition of symbol at position."),
			positionOp("implementation", "Find implementations of interface/method at position."),
			{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{"type": "string", "const": "diagnostics"},
				},
				"required":             []string{"operation"},
				"additionalProperties": false,
				"description":          "Get current diagnostics (errors, warnings) for edited files.",
			},
			{
				"type": "object",
				"properties": map[string]any{
					"operation": map[string]any{"type": "string", "const": "rename"},
					"file_path": map[string]any{"type": "string", "description": "Absolute or workspace-relative path to the file."},
					"line":      map[string]any{"type": "integer", "description": "1-based line number."},
					"column":    map[string]any{"type": "integer", "description": "1-based column number (byte offset in line)."},
					"new_name":  map[string]any{"type": "string", "description": "New name for the symbol."},
				},
				"required":             []string{"operation", "file_path", "line", "column", "new_name"},
				"additionalProperties": false,
				"description":          "Rename symbol across the project.",
			},
			positionOp("prepare_call_hierarchy", "Prepare call hierarchy item at position (first step for call hierarchy)."),
			positionOp("incoming_calls", "Find all callers of the function at position."),
			positionOp("outgoing_calls", "Find all functions called by the function at position."),
		},
	},
}

var LSPToolAgentSpec = tools.AgentToolSpec{
	Name:        "lsp",
	Version:     "1",
	Description: "Code intelligence via Language Server Protocol",
	RiskLevel:   tools.RiskLevelMedium,
	SideEffects: true, // rename writes files
}

func strPtr(s string) *string { return &s }
