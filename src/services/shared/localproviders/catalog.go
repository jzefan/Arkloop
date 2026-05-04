package localproviders

const (
	SourceLocal       = "local"
	OwnershipReadOnly = "read_only"

	AuthModeAPIKey = "api_key"
	AuthModeOAuth  = "oauth"

	ClaudeCodeProviderID   = "claude_code_local"
	ClaudeCodeDisplayName  = "Claude Code (Local)"
	ClaudeCodeProviderKind = "claude_code_local"

	CodexProviderID   = "codex_local"
	CodexDisplayName  = "Codex (Local)"
	CodexProviderKind = "codex_local"
)

type Model struct {
	ID              string
	ContextLength   int
	MaxOutputTokens int
	ToolCalling     bool
	Reasoning       bool
	Default         bool
	Priority        int
}

type ProviderStatus struct {
	ID          string
	DisplayName string
	Provider    string
	AuthMode    string
	Models      []Model
}

func ClaudeCodeModels() []Model {
	return []Model{
		{
			ID:              "claude-sonnet-4-6",
			ContextLength:   200000,
			MaxOutputTokens: 64000,
			ToolCalling:     true,
			Reasoning:       true,
			Default:         true,
			Priority:        900,
		},
		{
			ID:              "claude-sonnet-4-5",
			ContextLength:   200000,
			MaxOutputTokens: 64000,
			ToolCalling:     true,
			Reasoning:       true,
			Priority:        800,
		},
	}
}

func CodexModels() []Model {
	return []Model{
		{
			ID:              "gpt-5.3-codex",
			ContextLength:   400000,
			MaxOutputTokens: 128000,
			ToolCalling:     true,
			Reasoning:       true,
			Default:         true,
			Priority:        900,
		},
		{
			ID:              "gpt-5.3-codex-spark",
			ContextLength:   400000,
			MaxOutputTokens: 128000,
			ToolCalling:     true,
			Reasoning:       true,
			Priority:        800,
		},
		{
			ID:              "gpt-5.2-codex",
			ContextLength:   400000,
			MaxOutputTokens: 128000,
			ToolCalling:     true,
			Reasoning:       true,
			Priority:        700,
		},
	}
}

func modelByID(providerID string, modelID string) (Model, bool) {
	var models []Model
	switch providerID {
	case ClaudeCodeProviderID:
		models = ClaudeCodeModels()
	case CodexProviderID:
		models = CodexModels()
	default:
		return Model{}, false
	}
	for _, model := range models {
		if model.ID == modelID {
			return model, true
		}
	}
	return Model{}, false
}
