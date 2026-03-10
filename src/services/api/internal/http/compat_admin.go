package http

import (
	"time"

	sharedconfig "arkloop/services/shared/config"
	sharedexec "arkloop/services/shared/executionconfig"
)

type executionGovernanceResponse struct {
	Limits               []sharedconfig.SettingInspection `json:"limits"`
	TitleSummarizerModel *string                          `json:"title_summarizer_model,omitempty"`
	Personas             []executionGovernancePersona     `json:"personas"`
}

type executionGovernancePersona struct {
	ID                  string                              `json:"id"`
	Source              string                              `json:"source"`
	PersonaKey          string                              `json:"persona_key"`
	Version             string                              `json:"version"`
	DisplayName         string                              `json:"display_name"`
	PreferredCredential *string                             `json:"preferred_credential,omitempty"`
	Model               *string                             `json:"model,omitempty"`
	ReasoningMode       string                              `json:"reasoning_mode,omitempty"`
	PromptCacheControl  string                              `json:"prompt_cache_control,omitempty"`
	Requested           sharedexec.RequestedBudgets         `json:"requested"`
	Effective           executionGovernancePersonaEffective `json:"effective"`
}

type executionGovernancePersonaEffective struct {
	ReasoningIterations    int                          `json:"reasoning_iterations"`
	ToolContinuationBudget int                          `json:"tool_continuation_budget"`
	MaxOutputTokens        *int                         `json:"max_output_tokens,omitempty"`
	Temperature            *float64                     `json:"temperature,omitempty"`
	TopP                   *float64                     `json:"top_p,omitempty"`
	ReasoningMode          string                       `json:"reasoning_mode,omitempty"`
	PerToolSoftLimits      sharedexec.PerToolSoftLimits `json:"per_tool_soft_limits,omitempty"`
}

type broadcastResponse struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	TargetType  string         `json:"target_type"`
	TargetID    *string        `json:"target_id,omitempty"`
	PayloadJSON map[string]any `json:"payload"`
	Status      string         `json:"status"`
	SentCount   int            `json:"sent_count"`
	CreatedBy   string         `json:"created_by"`
	CreatedAt   string         `json:"created_at"`
}

type adminReportItem struct {
	ID            string   `json:"id"`
	ThreadID      string   `json:"thread_id"`
	ReporterID    string   `json:"reporter_id"`
	ReporterEmail string   `json:"reporter_email"`
	Categories    []string `json:"categories"`
	Feedback      *string  `json:"feedback"`
	CreatedAt     string   `json:"created_at"`
}

type adminReportsResponse struct {
	Data  []adminReportItem `json:"data"`
	Total int               `json:"total"`
}

type adminUserResponse struct {
	ID              string  `json:"id"`
	Login           *string `json:"login,omitempty"`
	Username        string  `json:"username"`
	Email           *string `json:"email"`
	EmailVerifiedAt *string `json:"email_verified_at,omitempty"`
	Status          string  `json:"status"`
	AvatarURL       *string `json:"avatar_url,omitempty"`
	Locale          *string `json:"locale,omitempty"`
	Timezone        *string `json:"timezone,omitempty"`
	LastLoginAt     *string `json:"last_login_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

type adminUserDetailResponse struct {
	adminUserResponse
	Orgs []adminUserOrgResponse `json:"orgs"`
}

type adminUserOrgResponse struct {
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
}

var _ = time.RFC3339
