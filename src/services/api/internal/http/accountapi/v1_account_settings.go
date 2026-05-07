package accountapi

import (
	"encoding/json"
	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	httpkit "arkloop/services/api/internal/http/httpkit"
	"arkloop/services/api/internal/observability"
)

const (
	pipelineTraceEnabledSettingKey    = "pipeline_trace_enabled"
	promptCacheDebugEnabledSettingKey = "prompt_cache_debug_enabled"
)

type accountSettingsResponse struct {
	PipelineTraceEnabled    bool `json:"pipeline_trace_enabled"`
	PromptCacheDebugEnabled bool `json:"prompt_cache_debug_enabled"`
}

type patchAccountSettingsRequest struct {
	PipelineTraceEnabled    *bool `json:"pipeline_trace_enabled"`
	PromptCacheDebugEnabled *bool `json:"prompt_cache_debug_enabled"`
}

func accountSettingsEntry(
	authService *auth.Service,
	membershipRepo *data.AccountMembershipRepository,
	accountRepo *data.AccountRepository,
	apiKeysRepo *data.APIKeysRepository,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		if accountRepo == nil {
			httpkit.WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
			return
		}

		actor, ok := httpkit.ResolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}

		switch r.Method {
		case nethttp.MethodGet:
			account, err := accountRepo.GetByID(r.Context(), actor.AccountID)
			if err != nil {
				httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			if account == nil {
				httpkit.WriteNotFound(w, r)
				return
			}
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, accountSettingsResponse{
				PipelineTraceEnabled:    pipelineTraceEnabledFromJSON(account.SettingsJSON),
				PromptCacheDebugEnabled: promptCacheDebugEnabledFromJSON(account.SettingsJSON),
			})
		case nethttp.MethodPatch:
			var body patchAccountSettingsRequest
			if err := httpkit.DecodeJSON(r, &body); err != nil {
				httpkit.WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid request body", traceID, nil)
				return
			}
			if body.PipelineTraceEnabled == nil && body.PromptCacheDebugEnabled == nil {
				httpkit.WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "at least one setting is required", traceID, nil)
				return
			}
			if body.PipelineTraceEnabled != nil {
				if err := accountRepo.UpdateSettings(r.Context(), actor.AccountID, pipelineTraceEnabledSettingKey, *body.PipelineTraceEnabled); err != nil {
					httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
					return
				}
			}
			if body.PromptCacheDebugEnabled != nil {
				if err := accountRepo.UpdateSettings(r.Context(), actor.AccountID, promptCacheDebugEnabledSettingKey, *body.PromptCacheDebugEnabled); err != nil {
					httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
					return
				}
			}
			account, err := accountRepo.GetByID(r.Context(), actor.AccountID)
			if err != nil || account == nil {
				httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, accountSettingsResponse{
				PipelineTraceEnabled:    boolFromJSON(account.SettingsJSON, pipelineTraceEnabledSettingKey),
				PromptCacheDebugEnabled: boolFromJSON(account.SettingsJSON, promptCacheDebugEnabledSettingKey),
			})
		default:
			httpkit.WriteMethodNotAllowed(w, r)
		}
	}
}

func pipelineTraceEnabledFromJSON(raw json.RawMessage) bool {
	return boolFromJSON(raw, pipelineTraceEnabledSettingKey)
}

func promptCacheDebugEnabledFromJSON(raw json.RawMessage) bool {
	return boolFromJSON(raw, promptCacheDebugEnabledSettingKey)
}

func boolFromJSON(raw json.RawMessage, key string) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	value, _ := payload[key].(bool)
	return value
}
