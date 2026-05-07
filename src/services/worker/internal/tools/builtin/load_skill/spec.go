package loadskill

import (
	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "load_skill"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "load an available skill into the current conversation by skill name",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: false,
}

var LlmSpec = llm.ToolSpec{
	Name:        ToolName,
	Description: strPtr(sharedtoolmeta.Must(ToolName).LLMDescription),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": "skill name from <available_skills>",
			},
		},
		"required":             []string{"skill"},
		"additionalProperties": false,
	},
}

func strPtr(value string) *string { return &value }
