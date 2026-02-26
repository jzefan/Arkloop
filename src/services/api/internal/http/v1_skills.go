package http

import (
	"encoding/json"
	"errors"
	"strings"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type createSkillRequest struct {
	SkillKey            string          `json:"skill_key"`
	Version             string          `json:"version"`
	DisplayName         string          `json:"display_name"`
	Description         *string         `json:"description"`
	PromptMD            string          `json:"prompt_md"`
	ToolAllowlist       []string        `json:"tool_allowlist"`
	BudgetsJSON         json.RawMessage `json:"budgets"`
	PreferredCredential *string         `json:"preferred_credential"`
}

type patchSkillRequest struct {
	DisplayName         *string         `json:"display_name"`
	Description         *string         `json:"description"`
	PromptMD            *string         `json:"prompt_md"`
	ToolAllowlist       []string        `json:"tool_allowlist"`
	BudgetsJSON         json.RawMessage `json:"budgets"`
	IsActive            *bool           `json:"is_active"`
	PreferredCredential *string         `json:"preferred_credential"`
}

type skillResponse struct {
	ID                  string          `json:"id"`
	OrgID               *string         `json:"org_id"`
	SkillKey            string          `json:"skill_key"`
	Version             string          `json:"version"`
	DisplayName         string          `json:"display_name"`
	Description         *string         `json:"description,omitempty"`
	PromptMD            string          `json:"prompt_md"`
	ToolAllowlist       []string        `json:"tool_allowlist"`
	BudgetsJSON         json.RawMessage `json:"budgets"`
	IsActive            bool            `json:"is_active"`
	CreatedAt           string          `json:"created_at"`
	PreferredCredential *string         `json:"preferred_credential,omitempty"`
}

func skillsEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	skillsRepo *data.SkillsRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		switch r.Method {
		case nethttp.MethodPost:
			createSkill(w, r, traceID, authService, membershipRepo, skillsRepo)
		case nethttp.MethodGet:
			listSkills(w, r, traceID, authService, membershipRepo, skillsRepo)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func skillEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	skillsRepo *data.SkillsRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		tail := strings.TrimPrefix(r.URL.Path, "/v1/skills/")
		tail = strings.Trim(tail, "/")
		if tail == "" {
			writeNotFound(w, r)
			return
		}

		skillID, err := uuid.Parse(tail)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
			return
		}

		switch r.Method {
		case nethttp.MethodPatch:
			patchSkill(w, r, traceID, skillID, authService, membershipRepo, skillsRepo)
		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func createSkill(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	skillsRepo *data.SkillsRepository,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if skillsRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	var req createSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return
	}

	req.SkillKey = strings.TrimSpace(req.SkillKey)
	req.Version = strings.TrimSpace(req.Version)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.PromptMD = strings.TrimSpace(req.PromptMD)

	if req.SkillKey == "" || req.Version == "" || req.DisplayName == "" || req.PromptMD == "" {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "skill_key, version, display_name, and prompt_md are required", traceID, nil)
		return
	}

	skill, err := skillsRepo.Create(
		r.Context(),
		actor.OrgID,
		req.SkillKey,
		req.Version,
		req.DisplayName,
		req.Description,
		req.PromptMD,
		req.ToolAllowlist,
		req.BudgetsJSON,
		req.PreferredCredential,
	)
	if err != nil {
		var conflict data.SkillConflictError
		if errors.As(err, &conflict) {
			WriteError(w, nethttp.StatusConflict, "skills.conflict", "skill with this key and version already exists", traceID, nil)
			return
		}
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	writeJSON(w, traceID, nethttp.StatusCreated, toSkillResponse(skill))
}

func listSkills(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	skillsRepo *data.SkillsRepository,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if skillsRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	skills, err := skillsRepo.ListByOrg(r.Context(), actor.OrgID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}

	resp := make([]skillResponse, 0, len(skills))
	for _, s := range skills {
		resp = append(resp, toSkillResponse(s))
	}

	writeJSON(w, traceID, nethttp.StatusOK, resp)
}

func patchSkill(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	skillID uuid.UUID,
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	skillsRepo *data.SkillsRepository,
) {
	if authService == nil {
		writeAuthNotConfigured(w, traceID)
		return
	}
	if skillsRepo == nil {
		WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
		return
	}

	actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
	if !ok {
		return
	}

	existing, err := skillsRepo.GetByID(r.Context(), actor.OrgID, skillID)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}
	if existing == nil {
		WriteError(w, nethttp.StatusNotFound, "skills.not_found", "skill not found", traceID, nil)
		return
	}

	var req patchSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "request validation failed", traceID, nil)
		return
	}

	patch := data.SkillPatch{
		DisplayName:         req.DisplayName,
		Description:         req.Description,
		PromptMD:            req.PromptMD,
		ToolAllowlist:       req.ToolAllowlist,
		BudgetsJSON:         req.BudgetsJSON,
		IsActive:            req.IsActive,
		PreferredCredential: req.PreferredCredential,
	}

	updated, err := skillsRepo.Patch(r.Context(), actor.OrgID, skillID, patch)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}
	if updated == nil {
		WriteError(w, nethttp.StatusNotFound, "skills.not_found", "skill not found", traceID, nil)
		return
	}

	writeJSON(w, traceID, nethttp.StatusOK, toSkillResponse(*updated))
}

func toSkillResponse(s data.Skill) skillResponse {
	allowlist := s.ToolAllowlist
	if allowlist == nil {
		allowlist = []string{}
	}

	budgets := s.BudgetsJSON
	if len(budgets) == 0 {
		budgets = json.RawMessage("{}")
	}

	var orgIDStr *string
	if s.OrgID != nil {
		str := s.OrgID.String()
		orgIDStr = &str
	}

	return skillResponse{
		ID:                  s.ID.String(),
		OrgID:               orgIDStr,
		SkillKey:            s.SkillKey,
		Version:             s.Version,
		DisplayName:         s.DisplayName,
		Description:         s.Description,
		PromptMD:            s.PromptMD,
		ToolAllowlist:       allowlist,
		BudgetsJSON:         budgets,
		IsActive:            s.IsActive,
		CreatedAt:           s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		PreferredCredential: s.PreferredCredential,
	}
}
