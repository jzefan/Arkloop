package http

import (
	"strings"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
)

type adminRunEventsStats struct {
	Total             int `json:"total"`
	LlmTurns          int `json:"llm_turns"`
	ToolCalls         int `json:"tool_calls"`
	ProviderFallbacks int `json:"provider_fallbacks"`
}

type adminRunDetailResponse struct {
	RunID             string   `json:"run_id"`
	OrgID             string   `json:"org_id"`
	ThreadID          string   `json:"thread_id"`
	Status            string   `json:"status"`
	Model             *string  `json:"model,omitempty"`
	SkillID           *string  `json:"skill_id,omitempty"`
	ProviderKind      *string  `json:"provider_kind,omitempty"`
	APIMode           *string  `json:"api_mode,omitempty"`
	RouteID           *string  `json:"route_id,omitempty"`
	CredentialID      *string  `json:"credential_id,omitempty"`
	DurationMs        *int64   `json:"duration_ms,omitempty"`
	TotalInputTokens  *int64   `json:"total_input_tokens,omitempty"`
	TotalOutputTokens *int64   `json:"total_output_tokens,omitempty"`
	TotalCostUSD      *float64 `json:"total_cost_usd,omitempty"`
	CreatedAt         string   `json:"created_at"`
	CompletedAt       *string  `json:"completed_at,omitempty"`
	FailedAt          *string  `json:"failed_at,omitempty"`
	// 创建者
	CreatedByUserID   *string `json:"created_by_user_id,omitempty"`
	CreatedByUserName *string `json:"created_by_user_name,omitempty"`
	CreatedByEmail    *string `json:"created_by_email,omitempty"`
	// 触发该 run 的用户 prompt（来自 messages 表）
	UserPrompt *string `json:"user_prompt,omitempty"`
	// 事件统计
	EventsStats adminRunEventsStats `json:"events_stats"`
}

func adminRunsEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	runRepo *data.RunEventRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
	messagesRepo *data.MessageRepository,
) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		if authService == nil {
			writeAuthNotConfigured(w, traceID)
			return
		}
		if runRepo == nil || usersRepo == nil {
			WriteError(w, nethttp.StatusServiceUnavailable, "database.not_configured", "database not configured", traceID, nil)
			return
		}

		actor, ok := resolveActor(w, r, traceID, authService, membershipRepo, apiKeysRepo, nil)
		if !ok {
			return
		}
		if !requirePerm(actor, auth.PermPlatformAdmin, w, traceID) {
			return
		}

		// 路径：/v1/admin/runs/{run_id}
		tail := strings.TrimPrefix(r.URL.Path, "/v1/admin/runs/")
		tail = strings.Trim(tail, "/")
		if tail == "" {
			writeNotFound(w, r)
			return
		}

		runID, err := uuid.Parse(tail)
		if err != nil {
			WriteError(w, nethttp.StatusUnprocessableEntity, "validation.error", "invalid run_id", traceID, nil)
			return
		}

		if r.Method != nethttp.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}

		run, err := runRepo.GetRun(r.Context(), runID)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if run == nil {
			WriteError(w, nethttp.StatusNotFound, "runs.not_found", "run not found", traceID, nil)
			return
		}

		// 取事件流，用于统计和提取 provider 信息
		events, err := runRepo.ListEvents(r.Context(), runID, 0, 2000)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}

		stats, providerKind, apiMode, routeID, credentialID := summarizeRunEvents(events)

		resp := adminRunDetailResponse{
			RunID:             run.ID.String(),
			OrgID:             run.OrgID.String(),
			ThreadID:          run.ThreadID.String(),
			Status:            run.Status,
			Model:             run.Model,
			SkillID:           run.SkillID,
			ProviderKind:      providerKind,
			APIMode:           apiMode,
			RouteID:           routeID,
			CredentialID:      credentialID,
			DurationMs:        run.DurationMs,
			TotalInputTokens:  run.TotalInputTokens,
			TotalOutputTokens: run.TotalOutputTokens,
			TotalCostUSD:      run.TotalCostUSD,
			CreatedAt:         run.CreatedAt.UTC().Format(time.RFC3339Nano),
			EventsStats:       stats,
		}

		if run.CompletedAt != nil {
			s := run.CompletedAt.UTC().Format(time.RFC3339Nano)
			resp.CompletedAt = &s
		}
		if run.FailedAt != nil {
			s := run.FailedAt.UTC().Format(time.RFC3339Nano)
			resp.FailedAt = &s
		}
		if run.CreatedByUserID != nil {
			s := run.CreatedByUserID.String()
			resp.CreatedByUserID = &s

			user, err := usersRepo.GetByID(r.Context(), *run.CreatedByUserID)
			if err == nil && user != nil {
				resp.CreatedByUserName = &user.DisplayName
				resp.CreatedByEmail = user.Email
			}
		}

		// 从 messages 表找触发该 run 的最后一条用户消息
		if messagesRepo != nil {
			msgs, mErr := messagesRepo.ListByThread(r.Context(), run.OrgID, run.ThreadID, 200)
			if mErr == nil {
				for i := len(msgs) - 1; i >= 0; i-- {
					m := msgs[i]
					if m.Role == "user" && !m.CreatedAt.After(run.CreatedAt) {
						resp.UserPrompt = &m.Content
						break
					}
				}
			}
		}

		writeJSON(w, traceID, nethttp.StatusOK, resp)
	}
}

// summarizeRunEvents 遍历事件流，统计各类事件数量，并提取 provider/route/credential 信息。
func summarizeRunEvents(events []data.RunEvent) (stats adminRunEventsStats, providerKind *string, apiMode *string, routeID *string, credentialID *string) {
	stats.Total = len(events)
	for _, ev := range events {
		switch ev.Type {
		case "run.route.selected":
			// 优先从路由事件提取，不依赖 llm.request（需开启 debug events）
			if routeID == nil {
				if r, ok := stringFromData(ev.DataJSON, "route_id"); ok {
					routeID = &r
				}
			}
			if credentialID == nil {
				if c, ok := stringFromData(ev.DataJSON, "credential_id"); ok {
					credentialID = &c
				}
			}
			if providerKind == nil {
				if pk, ok := stringFromData(ev.DataJSON, "provider_kind"); ok {
					providerKind = &pk
				}
			}
		case "llm.request":
			stats.LlmTurns++
			if providerKind == nil {
				if pk, ok := stringFromData(ev.DataJSON, "provider_kind"); ok {
					providerKind = &pk
				}
				if am, ok := stringFromData(ev.DataJSON, "api_mode"); ok {
					apiMode = &am
				}
			}
		case "tool.call":
			stats.ToolCalls++
		case "run.provider_fallback":
			stats.ProviderFallbacks++
		}
	}
	// EmitDebugEvents=false 时 llm.request 不存在，通过 message.delta / tool.result 状态机推断轮次
	if stats.LlmTurns == 0 {
		type phase int
		const (
			phaseIdle  phase = iota
			phaseInLLM       // 正在接收 LLM 输出
			phaseInTools     // 工具执行中，等待下一轮 LLM
		)
		p := phaseIdle
		for _, ev := range events {
			switch ev.Type {
			case "message.delta":
				if p == phaseIdle {
					stats.LlmTurns++
					p = phaseInLLM
				} else if p == phaseInTools {
					stats.LlmTurns++
					p = phaseInLLM
				}
			case "tool.result":
				if p == phaseInLLM {
					p = phaseInTools
				}
			}
		}
	}
	return stats, providerKind, apiMode, routeID, credentialID
}

func stringFromData(dataJSON any, key string) (string, bool) {
	m, ok := dataJSON.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
