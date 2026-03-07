package http

import (
	"strings"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type toolCatalogItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	HasOverride bool   `json:"has_override"`
}

type toolCatalogGroup struct {
	Group string            `json:"group"`
	Tools []toolCatalogItem `json:"tools"`
}

type toolCatalogResponse struct {
	Groups []toolCatalogGroup `json:"groups"`
}

type defaultToolDef struct {
	Group       string
	Name        string
	Description string
}

var defaultToolDefs = []defaultToolDef{
	{Group: "web_search", Name: "web_search", Description: "Web search"},
	{Group: "web_fetch", Name: "web_fetch", Description: "Web page fetch"},
	{Group: "sandbox", Name: "python_execute", Description: "Python code execution"},
	{Group: "sandbox", Name: "exec_command", Description: "Persistent shell command execution"},
	{Group: "sandbox", Name: "write_stdin", Description: "Shell stdin and poll"},
	{Group: "memory", Name: "memory_search", Description: "Search user memory"},
	{Group: "memory", Name: "memory_commit", Description: "Commit to user memory"},
	{Group: "browser", Name: "browser_navigate", Description: "Navigate to URL"},
	{Group: "browser", Name: "browser_interact", Description: "Interact with page element"},
	{Group: "browser", Name: "browser_screenshot", Description: "Take page screenshot"},
}

func buildToolCatalog(overrides []data.ToolDescriptionOverride) toolCatalogResponse {
	byName := map[string]string{}
	for _, o := range overrides {
		byName[o.ToolName] = o.Description
	}

	groupOrder := []string{"web_search", "web_fetch", "sandbox", "memory", "browser"}
	grouped := map[string][]toolCatalogItem{}
	for _, def := range defaultToolDefs {
		desc := def.Description
		hasOverride := false
		if ov, ok := byName[def.Name]; ok {
			desc = ov
			hasOverride = true
		}
		grouped[def.Group] = append(grouped[def.Group], toolCatalogItem{
			Name:        def.Name,
			Description: desc,
			HasOverride: hasOverride,
		})
	}

	var groups []toolCatalogGroup
	for _, g := range groupOrder {
		if items, ok := grouped[g]; ok {
			groups = append(groups, toolCatalogGroup{Group: g, Tools: items})
		}
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
		if _, ok := authenticateActor(w, r, traceID, authService, membershipRepo); !ok {
			return
		}

		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		if scope == "" {
			scope = "platform"
		}

		var overrides []data.ToolDescriptionOverride
		if overridesRepo != nil {
			var err error
			overrides, err = overridesRepo.ListByScope(r.Context(), uuid.Nil, scope)
			if err != nil {
				overrides = nil
			}
		}

		writeJSON(w, traceID, nethttp.StatusOK, buildToolCatalog(overrides))
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
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		if scope == "" {
			scope = "platform"
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
			if err := overridesRepo.Upsert(r.Context(), uuid.Nil, scope, toolName, req.Description); err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			w.WriteHeader(nethttp.StatusNoContent)

		case nethttp.MethodDelete:
			if err := overridesRepo.Delete(r.Context(), uuid.Nil, scope, toolName); err != nil {
				WriteError(w, nethttp.StatusNotFound, "not_found", "no override found", traceID, nil)
				return
			}
			w.WriteHeader(nethttp.StatusNoContent)

		default:
			writeMethodNotAllowed(w, r)
		}
	}
}
