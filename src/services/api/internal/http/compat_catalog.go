package http

import (
	catalogfamily "arkloop/services/api/internal/http/catalogapi"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/personas"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type toolDescriptionSource string

const (
	toolDescriptionSourceDefault  toolDescriptionSource = "default"
	toolDescriptionSourcePlatform toolDescriptionSource = "platform"
	toolDescriptionSourceOrg      toolDescriptionSource = "org"

	anthropicAdvancedVersionKey      = "anthropic_version"
	anthropicAdvancedExtraHeadersKey = "extra_headers"
	anthropicBetaHeaderName          = "anthropic-beta"
)

type toolCatalogItem struct {
	Name              string                `json:"name"`
	Label             string                `json:"label"`
	LLMDescription    string                `json:"llm_description"`
	HasOverride       bool                  `json:"has_override"`
	DescriptionSource toolDescriptionSource `json:"description_source"`
	IsDisabled        bool                  `json:"is_disabled"`
}

type toolCatalogGroup struct {
	Group string            `json:"group"`
	Tools []toolCatalogItem `json:"tools"`
}

type toolCatalogResponse struct {
	Groups []toolCatalogGroup `json:"groups"`
}

type llmProviderResponse struct {
	ID            string                     `json:"id"`
	OrgID         string                     `json:"org_id"`
	Provider      string                     `json:"provider"`
	Name          string                     `json:"name"`
	KeyPrefix     *string                    `json:"key_prefix"`
	BaseURL       *string                    `json:"base_url"`
	OpenAIAPIMode *string                    `json:"openai_api_mode"`
	AdvancedJSON  map[string]any             `json:"advanced_json,omitempty"`
	CreatedAt     string                     `json:"created_at"`
	Models        []llmProviderModelResponse `json:"models"`
}

type llmProviderModelResponse struct {
	ID                  string          `json:"id"`
	ProviderID          string          `json:"provider_id"`
	Model               string          `json:"model"`
	Priority            int             `json:"priority"`
	IsDefault           bool            `json:"is_default"`
	Tags                []string        `json:"tags"`
	WhenJSON            json.RawMessage `json:"when"`
	AdvancedJSON        map[string]any  `json:"advanced_json,omitempty"`
	Multiplier          float64         `json:"multiplier"`
	CostPer1kInput      *float64        `json:"cost_per_1k_input,omitempty"`
	CostPer1kOutput     *float64        `json:"cost_per_1k_output,omitempty"`
	CostPer1kCacheWrite *float64        `json:"cost_per_1k_cache_write,omitempty"`
	CostPer1kCacheRead  *float64        `json:"cost_per_1k_cache_read,omitempty"`
}

type llmProviderAvailableModelsResponse struct {
	Models []llmProviderAvailableModel `json:"models"`
}

type llmProviderAvailableModel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

type personaResponse struct {
	ID                  string          `json:"id"`
	OrgID               *string         `json:"org_id"`
	PersonaKey          string          `json:"persona_key"`
	Version             string          `json:"version"`
	DisplayName         string          `json:"display_name"`
	Description         *string         `json:"description,omitempty"`
	UserSelectable      bool            `json:"user_selectable"`
	SelectorName        *string         `json:"selector_name,omitempty"`
	SelectorOrder       *int            `json:"selector_order,omitempty"`
	PromptMD            string          `json:"prompt_md"`
	ToolAllowlist       []string        `json:"tool_allowlist"`
	ToolDenylist        []string        `json:"tool_denylist"`
	BudgetsJSON         json.RawMessage `json:"budgets"`
	IsActive            bool            `json:"is_active"`
	CreatedAt           string          `json:"created_at"`
	PreferredCredential *string         `json:"preferred_credential,omitempty"`
	Model               *string         `json:"model,omitempty"`
	ReasoningMode       string          `json:"reasoning_mode"`
	PromptCacheControl  string          `json:"prompt_cache_control"`
	ExecutorType        string          `json:"executor_type"`
	ExecutorConfigJSON  json.RawMessage `json:"executor_config"`
	Source              string          `json:"source"`
}

type liteAgentResponse struct {
	ID              string          `json:"id"`
	PersonaKey      string          `json:"persona_key"`
	DisplayName     string          `json:"display_name"`
	Description     *string         `json:"description,omitempty"`
	PromptMD        string          `json:"prompt_md"`
	Model           *string         `json:"model,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	ReasoningMode   string          `json:"reasoning_mode"`
	ToolPolicy      string          `json:"tool_policy"`
	ToolAllowlist   []string        `json:"tool_allowlist"`
	ToolDenylist    []string        `json:"tool_denylist"`
	IsActive        bool            `json:"is_active"`
	ExecutorType    string          `json:"executor_type"`
	BudgetsJSON     json.RawMessage `json:"budgets"`
	Source          string          `json:"source"`
	CreatedAt       string          `json:"created_at"`
}

func validateAdvancedJSONForProvider(provider string, advancedJSON map[string]any) error {
	if strings.TrimSpace(provider) != "anthropic" || advancedJSON == nil {
		return nil
	}
	return validateAnthropicAdvancedJSON(advancedJSON)
}

func validateAnthropicAdvancedJSON(advancedJSON map[string]any) error {
	if advancedJSON == nil {
		return nil
	}
	if rawVersion, ok := advancedJSON[anthropicAdvancedVersionKey]; ok {
		version, ok := rawVersion.(string)
		if !ok || strings.TrimSpace(version) == "" {
			return errors.New("advanced_json.anthropic_version must be a non-empty string")
		}
	}

	rawHeaders, ok := advancedJSON[anthropicAdvancedExtraHeadersKey]
	if !ok {
		return nil
	}
	headers, ok := rawHeaders.(map[string]any)
	if !ok {
		return errors.New("advanced_json.extra_headers must be an object")
	}
	for key, value := range headers {
		headerName := strings.ToLower(strings.TrimSpace(key))
		if headerName != anthropicBetaHeaderName {
			return errors.New("advanced_json.extra_headers only supports anthropic-beta")
		}
		headerValue, ok := value.(string)
		if !ok || strings.TrimSpace(headerValue) == "" {
			return errors.New("advanced_json.extra_headers.anthropic-beta must be a non-empty string")
		}
	}
	return nil
}

func toLiteAgentFromDB(p data.Persona) liteAgentResponse {
	allowlist := p.ToolAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	denylist := p.ToolDenylist
	if denylist == nil {
		denylist = []string{}
	}
	budgets := p.BudgetsJSON
	if len(budgets) == 0 {
		budgets = json.RawMessage("{}")
	}
	executorType := strings.TrimSpace(p.ExecutorType)
	if executorType == "" {
		executorType = "agent.simple"
	}
	temperature, maxOutputTokens := extractLiteAgentBudgetValues(budgets)
	reasoningMode := strings.TrimSpace(p.ReasoningMode)
	if reasoningMode == "" {
		reasoningMode = "auto"
	}
	toolPolicy := "none"
	if len(allowlist) > 0 {
		toolPolicy = "allowlist"
	} else if len(denylist) > 0 {
		toolPolicy = "denylist"
	}
	return liteAgentResponse{
		ID:              p.ID.String(),
		PersonaKey:      p.PersonaKey,
		DisplayName:     p.DisplayName,
		Description:     p.Description,
		PromptMD:        p.PromptMD,
		Model:           optionalLiteTrimmedStringPtr(p.Model),
		Temperature:     temperature,
		MaxOutputTokens: maxOutputTokens,
		ReasoningMode:   reasoningMode,
		ToolPolicy:      toolPolicy,
		ToolAllowlist:   allowlist,
		ToolDenylist:    denylist,
		IsActive:        p.IsActive,
		ExecutorType:    executorType,
		BudgetsJSON:     budgets,
		Source:          "db",
		CreatedAt:       p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func toLiteAgentFromRepo(rp personas.RepoPersona) liteAgentResponse {
	allowlist := rp.ToolAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}
	denylist := rp.ToolDenylist
	if denylist == nil {
		denylist = []string{}
	}
	budgets := json.RawMessage("{}")
	if rp.Budgets != nil {
		if b, err := json.Marshal(rp.Budgets); err == nil {
			budgets = b
		}
	}
	executorType := strings.TrimSpace(rp.ExecutorType)
	if executorType == "" {
		executorType = "agent.simple"
	}
	temperature, maxOutputTokens := extractLiteAgentBudgetValues(budgets)
	reasoningMode := strings.TrimSpace(rp.ReasoningMode)
	if reasoningMode == "" {
		reasoningMode = "auto"
	}
	toolPolicy := "none"
	if len(allowlist) > 0 {
		toolPolicy = "allowlist"
	} else if len(denylist) > 0 {
		toolPolicy = "denylist"
	}
	return liteAgentResponse{
		ID:              rp.ID,
		PersonaKey:      rp.ID,
		DisplayName:     rp.Title,
		Description:     optionalLiteTrimmedString(rp.Description),
		PromptMD:        rp.PromptMD,
		Model:           optionalLiteTrimmedString(rp.Model),
		Temperature:     temperature,
		MaxOutputTokens: maxOutputTokens,
		ReasoningMode:   reasoningMode,
		ToolPolicy:      toolPolicy,
		ToolAllowlist:   allowlist,
		ToolDenylist:    denylist,
		IsActive:        true,
		ExecutorType:    executorType,
		BudgetsJSON:     budgets,
		Source:          "repo",
		CreatedAt:       "",
	}
}

func extractLiteAgentBudgetValues(raw json.RawMessage) (*float64, *int) {
	if len(raw) == 0 {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil
	}
	var temperature *float64
	if value, ok := payload["temperature"].(float64); ok {
		temperature = &value
	}
	var maxOutputTokens *int
	switch value := payload["max_output_tokens"].(type) {
	case float64:
		converted := int(value)
		maxOutputTokens = &converted
	case int:
		maxOutputTokens = &value
	}
	return temperature, maxOutputTokens
}

func optionalLiteTrimmedStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return optionalLiteTrimmedString(*value)
}

func optionalLiteTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func buildEffectiveToolCatalog(
	ctx context.Context,
	orgID uuid.UUID,
	overridesRepo *data.ToolDescriptionOverridesRepository,
	pool *pgxpool.Pool,
	mcpCache *catalogfamily.EffectiveToolCatalogCache,
	artifactStoreAvailable bool,
) (toolCatalogResponse, error) {
	resp, err := catalogfamily.BuildEffectiveToolCatalogCompat(ctx, orgID, overridesRepo, pool, mcpCache, artifactStoreAvailable)
	if err != nil {
		return toolCatalogResponse{}, err
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		return toolCatalogResponse{}, err
	}
	var converted toolCatalogResponse
	if err := json.Unmarshal(raw, &converted); err != nil {
		return toolCatalogResponse{}, err
	}
	return converted, nil
}
