package http

import (
	"strings"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"
	sharedtoolmeta "arkloop/services/shared/toolmeta"

	nethttp "net/http"

	"github.com/google/uuid"
)

type toolDescriptionSource string

const (
	toolDescriptionSourceDefault  toolDescriptionSource = "default"
	toolDescriptionSourcePlatform toolDescriptionSource = "platform"
	toolDescriptionSourceOrg      toolDescriptionSource = "org"
)

type toolCatalogItem struct {
	Name              string                `json:"name"`
	Label             string                `json:"label"`
	LLMDescription    string                `json:"llm_description"`
	HasOverride       bool                  `json:"has_override"`
	DescriptionSource toolDescriptionSource `json:"description_source"`
}

type toolCatalogGroup struct {
	Group string            `json:"group"`
	Tools []toolCatalogItem `json:"tools"`
}

type toolCatalogResponse struct {
	Groups []toolCatalogGroup `json:"groups"`
}

func buildToolCatalog(
	scope string,
	platformOverrides []data.ToolDescriptionOverride,
	orgOverrides []data.ToolDescriptionOverride,
) toolCatalogResponse {
	platformByName := buildToolDescriptionOverrideMap(platformOverrides)
	orgByName := buildToolDescriptionOverrideMap(orgOverrides)

	groups := make([]toolCatalogGroup, 0, len(sharedtoolmeta.GroupOrder()))
	for _, group := range sharedtoolmeta.Catalog() {
		items := make([]toolCatalogItem, 0, len(group.Tools))
		for _, meta := range group.Tools {
			description := meta.LLMDescription
			hasOverride := false
			source := toolDescriptionSourceDefault

			if scope == "org" {
				if override, ok := orgByName[meta.Name]; ok {
					description = override
					hasOverride = true
					source = toolDescriptionSourceOrg
				} else if override, ok := platformByName[meta.Name]; ok {
					description = override
					source = toolDescriptionSourcePlatform
				}
			} else if override, ok := platformByName[meta.Name]; ok {
				description = override
				hasOverride = true
				source = toolDescriptionSourcePlatform
			}

			items = append(items, toolCatalogItem{
				Name:              meta.Name,
				Label:             meta.Label,
				LLMDescription:    description,
				HasOverride:       hasOverride,
				DescriptionSource: source,
			})
		}
		groups = append(groups, toolCatalogGroup{Group: group.Name, Tools: items})
	}
	return toolCatalogResponse{Groups: groups}
}

func toolCatalogEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	overridesRepo *data.ToolDescriptionOverridesRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		if r.Method != nethttp.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}
		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}
		actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
		if !ok {
			return
		}

		scope, orgID, ok := resolveToolCatalogScope(w, r, traceID, actor)
		if !ok {
			return
		}

		var platformOverrides []data.ToolDescriptionOverride
		var orgOverrides []data.ToolDescriptionOverride
		if overridesRepo != nil {
			var err error
			platformOverrides, err = overridesRepo.ListByScope(r.Context(), uuid.Nil, "platform")
			if err != nil {
				platformOverrides = nil
			}
			if scope == "org" {
				orgOverrides, err = overridesRepo.ListByScope(r.Context(), orgID, "org")
				if err != nil {
					orgOverrides = nil
				}
			}
		}

		writeJSON(w, traceID, nethttp.StatusOK, buildToolCatalog(scope, platformOverrides, orgOverrides))
	}
}

type updateToolDescriptionRequest struct {
	Description string `json:"description"`
}

func toolCatalogItemEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	overridesRepo *data.ToolDescriptionOverridesRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		tail := strings.TrimPrefix(r.URL.Path, "/v1/tool-catalog/")
		parts := strings.SplitN(strings.Trim(tail, "/"), "/", 2)
		if len(parts) < 1 || parts[0] == "" {
			writeNotFound(w, r)
			return
		}
		toolName := parts[0]
		action := ""
		if len(parts) == 2 {
			action = parts[1]
		}
		if _, ok := sharedtoolmeta.Lookup(toolName); !ok {
			writeNotFound(w, r)
			return
		}
		if action != "description" {
			writeNotFound(w, r)
			return
		}

		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}
		actor, ok := authenticateActor(w, r, traceID, authService, membershipRepo)
		if !ok {
			return
		}

		scope, orgID, ok := resolveToolCatalogScope(w, r, traceID, actor)
		if !ok {
			return
		}

		if overridesRepo == nil {
			WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
			return
		}

		switch r.Method {
		case nethttp.MethodPut:
			var req updateToolDescriptionRequest
			if err := decodeJSON(r, &req); err != nil {
				WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid request body", traceID, nil)
				return
			}
			if strings.TrimSpace(req.Description) == "" {
				WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "description must not be empty", traceID, nil)
				return
			}
			if err := overridesRepo.Upsert(r.Context(), orgID, scope, toolName, req.Description); err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			w.WriteHeader(nethttp.StatusNoContent)

		case nethttp.MethodDelete:
			if err := overridesRepo.Delete(r.Context(), orgID, scope, toolName); err != nil {
				WriteError(w, nethttp.StatusNotFound, "not_found", "no override found", traceID, nil)
				return
			}
			w.WriteHeader(nethttp.StatusNoContent)

		default:
			writeMethodNotAllowed(w, r)
		}
	}
}

func resolveToolCatalogScope(
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	traceID string,
	actor *actor,
) (string, uuid.UUID, bool) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = "platform"
	}
	if scope != "org" && scope != "platform" {
		WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "scope must be org or platform", traceID, nil)
		return "", uuid.Nil, false
	}

	if scope == "platform" {
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return "", uuid.Nil, false
		}
		return scope, uuid.Nil, true
	}

	if !requirePerm(actor, auth.PermDataSecrets, w, traceID) {
		return "", uuid.Nil, false
	}
	return scope, actor.OrgID, true
}

func buildToolDescriptionOverrideMap(overrides []data.ToolDescriptionOverride) map[string]string {
	out := make(map[string]string, len(overrides))
	for _, override := range overrides {
		out[override.ToolName] = override.Description
	}
	return out
}
