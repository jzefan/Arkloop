package http

import (
	"context"
	"sort"
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

type adminRunUsageItem struct {
	RunID              string   `json:"run_id"`
	OrgID              string   `json:"org_id"`
	ThreadID           string   `json:"thread_id"`
	ParentRunID        *string  `json:"parent_run_id,omitempty"`
	Status             string   `json:"status"`
	PersonaID          *string  `json:"persona_id,omitempty"`
	Model              *string  `json:"model,omitempty"`
	ProviderKind       *string  `json:"provider_kind,omitempty"`
	CredentialName     *string  `json:"credential_name,omitempty"`
	AgentConfigName    *string  `json:"agent_config_name,omitempty"`
	DurationMs         *int64   `json:"duration_ms,omitempty"`
	TotalInputTokens   *int64   `json:"total_input_tokens,omitempty"`
	TotalOutputTokens  *int64   `json:"total_output_tokens,omitempty"`
	TotalCostUSD       *float64 `json:"total_cost_usd,omitempty"`
	CacheHitRate       *float64 `json:"cache_hit_rate,omitempty"`
	CacheCreationTokens *int64  `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens    *int64   `json:"cache_read_tokens,omitempty"`
	CachedTokens       *int64   `json:"cached_tokens,omitempty"`
	CreditsUsed        *int64   `json:"credits_used,omitempty"`
	CreatedAt          string   `json:"created_at"`
	CompletedAt        *string  `json:"completed_at,omitempty"`
	FailedAt           *string  `json:"failed_at,omitempty"`
}

type adminRunUsageAggregate struct {
	TotalInputTokens  *int64   `json:"total_input_tokens,omitempty"`
	TotalOutputTokens *int64   `json:"total_output_tokens,omitempty"`
	TotalCostUSD      *float64 `json:"total_cost_usd,omitempty"`
	CreditsUsed       *int64   `json:"credits_used,omitempty"`
}

type adminRunDetailResponse struct {
	RunID             string   `json:"run_id"`
	OrgID             string   `json:"org_id"`
	ThreadID          string   `json:"thread_id"`
	Status            string   `json:"status"`
	Model             *string  `json:"model,omitempty"`
	PersonaID           *string  `json:"persona_id,omitempty"`
	ProviderKind      *string  `json:"provider_kind,omitempty"`
	CredentialName    *string  `json:"credential_name,omitempty"`
	AgentConfigName   *string  `json:"agent_config_name,omitempty"`
	DurationMs        *int64   `json:"duration_ms,omitempty"`
	TotalInputTokens  *int64   `json:"total_input_tokens,omitempty"`
	TotalOutputTokens *int64   `json:"total_output_tokens,omitempty"`
	TotalCostUSD      *float64 `json:"total_cost_usd,omitempty"`
	CreatedAt         string   `json:"created_at"`
	CompletedAt       *string  `json:"completed_at,omitempty"`
	FailedAt          *string  `json:"failed_at,omitempty"`
	CreatedByUserID   *string  `json:"created_by_user_id,omitempty"`
	CreatedByUserName *string  `json:"created_by_user_name,omitempty"`
	CreatedByEmail    *string  `json:"created_by_email,omitempty"`
	UserPrompt        *string  `json:"user_prompt,omitempty"`
	EventsStats       adminRunEventsStats `json:"events_stats"`
	Children          []adminRunUsageItem `json:"children,omitempty"`
	TotalAggregate    *adminRunUsageAggregate `json:"total_aggregate,omitempty"`
}

func adminRunsEntry(
	authService *auth.Service,
	membershipRepo *data.OrgMembershipRepository,
	runRepo *data.RunEventRepository,
	usersRepo *data.UserRepository,
	apiKeysRepo *data.APIKeysRepository,
	messagesRepo *data.MessageRepository,
	credentialsRepo *data.LlmCredentialsRepository,
	agentConfigsRepo *data.AgentConfigRepository,
	threadRepo *data.ThreadRepository,
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

		stats, routeModel, providerKind, credentialID, credentialName, agentConfigName := summarizeRunEvents(events)

		// 如果事件中没有 credential_name（旧 run），尝试从 DB 查询补全
		if credentialName == nil && credentialID != nil && credentialsRepo != nil {
			if credUUID, err := uuid.Parse(*credentialID); err == nil {
				if cred, err := credentialsRepo.GetByID(r.Context(), run.OrgID, credUUID); err == nil && cred != nil {
					credentialName = &cred.Name
				}
			}
		}

		// model 优先用路由表里的实际模型名（如 gpt-4o），否则 fallback 到 runs 表
		model := routeModel
		if model == nil {
			model = run.Model
		}

		// 旧 run 事件中没有 agent_config_name 时，复用 Worker 的解析链：thread→project→org 默认
		if agentConfigName == nil && threadRepo != nil && agentConfigsRepo != nil {
			if thread, tErr := threadRepo.GetByID(r.Context(), run.ThreadID); tErr == nil && thread != nil {
				var resolvedID *uuid.UUID
				if thread.AgentConfigID != nil {
					resolvedID = thread.AgentConfigID
				} else {
					resolvedID = resolveDefaultAgentConfigID(r.Context(), agentConfigsRepo, run.OrgID, thread.ProjectID)
				}
				if resolvedID != nil {
					if ac, acErr := agentConfigsRepo.GetByID(r.Context(), *resolvedID); acErr == nil && ac != nil {
						agentConfigName = &ac.Name
					}
				}
			}
		}

		resp := adminRunDetailResponse{
			RunID:             run.ID.String(),
			OrgID:             run.OrgID.String(),
			ThreadID:          run.ThreadID.String(),
			Status:            run.Status,
			Model:             model,
			PersonaID:           run.PersonaID,
			ProviderKind:      providerKind,
			CredentialName:    credentialName,
			AgentConfigName:   agentConfigName,
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
				resp.CreatedByUserName = &user.Username
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

		// child runs usage breakdown（按模型拆分）
		childIDs, err := runRepo.ListChildRunIDs(r.Context(), runID)
		if err != nil {
			WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if len(childIDs) > 0 {
			// 复用 runs 列表查询的 join 口径，补齐 cache / credits 字段
			parentRow, err := loadRunUsageRow(r.Context(), runRepo, runID)
			if err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}
			childRows, err := loadChildRunUsageRows(r.Context(), runRepo, runID)
			if err != nil {
				WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
				return
			}

			children := make([]adminRunUsageItem, 0, len(childRows))
			agg := &adminRunUsageAggregate{}

			appendToAgg := func(item adminRunUsageItem) {
				if item.TotalInputTokens != nil {
					agg.TotalInputTokens = addInt64Ptr(agg.TotalInputTokens, *item.TotalInputTokens)
				}
				if item.TotalOutputTokens != nil {
					agg.TotalOutputTokens = addInt64Ptr(agg.TotalOutputTokens, *item.TotalOutputTokens)
				}
				if item.TotalCostUSD != nil {
					agg.TotalCostUSD = addFloat64Ptr(agg.TotalCostUSD, *item.TotalCostUSD)
				}
				if item.CreditsUsed != nil {
					agg.CreditsUsed = addInt64Ptr(agg.CreditsUsed, *item.CreditsUsed)
				}
			}

			if parentRow != nil {
				appendToAgg(*parentRow)
			}

			// created_at 升序，符合执行顺序
			sort.Slice(childRows, func(i, j int) bool {
				return childRows[i].CreatedAt < childRows[j].CreatedAt
			})

			for _, row := range childRows {
				children = append(children, row)
				appendToAgg(row)
			}

			resp.Children = children
			resp.TotalAggregate = agg
		}

		writeJSON(w, traceID, nethttp.StatusOK, resp)
	}
}

func addInt64Ptr(dst *int64, v int64) *int64 {
	if dst == nil {
		x := v
		return &x
	}
	*dst += v
	return dst
}

func addFloat64Ptr(dst *float64, v float64) *float64 {
	if dst == nil {
		x := v
		return &x
	}
	*dst += v
	return dst
}

func loadRunUsageRow(ctx context.Context, repo *data.RunEventRepository, runID uuid.UUID) (*adminRunUsageItem, error) {
	if repo == nil || runID == uuid.Nil {
		return nil, nil
	}
	params := data.ListRunsParams{RunID: &runID, Limit: 1, Offset: 0}
	rows, _, err := repo.ListRuns(ctx, params)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	item := toAdminRunUsageItem(rows[0], nil)
	_ = fillAdminRunUsageMeta(ctx, repo, runID, item)
	return item, nil
}

func loadChildRunUsageRows(ctx context.Context, repo *data.RunEventRepository, parentRunID uuid.UUID) ([]adminRunUsageItem, error) {
	if repo == nil || parentRunID == uuid.Nil {
		return nil, nil
	}
	params := data.ListRunsParams{ParentRunID: &parentRunID, Limit: 200, Offset: 0}
	rows, _, err := repo.ListRuns(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]adminRunUsageItem, 0, len(rows))
	parentID := parentRunID.String()
	for _, r := range rows {
		item := toAdminRunUsageItem(r, &parentID)
		_ = fillAdminRunUsageMeta(ctx, repo, r.ID, item)
		out = append(out, *item)
	}
	return out, nil
}

func toAdminRunUsageItem(rw data.RunWithUser, parentRunID *string) *adminRunUsageItem {
	item := &adminRunUsageItem{
		RunID:             rw.ID.String(),
		OrgID:             rw.OrgID.String(),
		ThreadID:          rw.ThreadID.String(),
		ParentRunID:       parentRunID,
		Status:            rw.Status,
		Model:             rw.Model,
		PersonaID:         rw.PersonaID,
		DurationMs:        rw.DurationMs,
		TotalInputTokens:  rw.TotalInputTokens,
		TotalOutputTokens: rw.TotalOutputTokens,
		TotalCostUSD:      rw.TotalCostUSD,
		CacheCreationTokens: rw.CacheCreationTokens,
		CacheReadTokens:   rw.CacheReadTokens,
		CachedTokens:      rw.CachedTokens,
		CreditsUsed:       rw.CreditsUsed,
		CreatedAt:         rw.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if rw.CompletedAt != nil {
		s := rw.CompletedAt.UTC().Format(time.RFC3339Nano)
		item.CompletedAt = &s
	}
	if rw.FailedAt != nil {
		s := rw.FailedAt.UTC().Format(time.RFC3339Nano)
		item.FailedAt = &s
	}
	item.CacheHitRate = calcCacheHitRate(rw.TotalInputTokens, rw.CacheReadTokens, rw.CacheCreationTokens, rw.CachedTokens)
	return item
}

func fillAdminRunUsageMeta(
	ctx context.Context,
	repo *data.RunEventRepository,
	runID uuid.UUID,
	item *adminRunUsageItem,
) error {
	if repo == nil || item == nil || runID == uuid.Nil {
		return nil
	}
	events, err := repo.ListEvents(ctx, runID, 0, 200)
	if err != nil {
		return err
	}

	_, routeModel, providerKind, _, credentialName, agentConfigName := summarizeRunEvents(events)
	if routeModel != nil {
		item.Model = routeModel
	}
	item.ProviderKind = providerKind
	item.CredentialName = credentialName
	item.AgentConfigName = agentConfigName

	if item.PersonaID == nil {
		if pid := personaIDFromEvents(events); pid != "" {
			item.PersonaID = &pid
		}
	}
	return nil
}

func personaIDFromEvents(events []data.RunEvent) string {
	for _, ev := range events {
		if ev.Type != "run.started" {
			continue
		}
		if pid, ok := stringFromData(ev.DataJSON, "persona_id"); ok {
			return pid
		}
	}
	return ""
}

// summarizeRunEvents 遍历事件流，统计各类事件数量，并提取路由相关信息。
func summarizeRunEvents(events []data.RunEvent) (
	stats adminRunEventsStats,
	routeModel *string,
	providerKind *string,
	credentialID *string,
	credentialName *string,
	agentConfigName *string,
) {
	stats.Total = len(events)
	for _, ev := range events {
		switch ev.Type {
		case "run.route.selected":
			if routeModel == nil {
				if m, ok := stringFromData(ev.DataJSON, "model"); ok {
					routeModel = &m
				}
			}
			if credentialID == nil {
				if c, ok := stringFromData(ev.DataJSON, "credential_id"); ok {
					credentialID = &c
				}
			}
			if credentialName == nil {
				if n, ok := stringFromData(ev.DataJSON, "credential_name"); ok {
					credentialName = &n
				}
			}
			if providerKind == nil {
				if pk, ok := stringFromData(ev.DataJSON, "provider_kind"); ok {
					providerKind = &pk
				}
			}
			if agentConfigName == nil {
				if n, ok := stringFromData(ev.DataJSON, "agent_config_name"); ok {
					agentConfigName = &n
				}
			}
		case "llm.request":
			stats.LlmTurns++
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
	return stats, routeModel, providerKind, credentialID, credentialName, agentConfigName
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

// resolveDefaultAgentConfigID 按 project→org 优先级查找默认 agent config。
func resolveDefaultAgentConfigID(
	ctx context.Context,
	repo *data.AgentConfigRepository,
	orgID uuid.UUID,
	projectID *uuid.UUID,
) *uuid.UUID {
	if projectID != nil {
		if ac, err := repo.GetDefaultForProject(ctx, orgID, *projectID); err == nil && ac != nil {
			return &ac.ID
		}
	}
	if ac, err := repo.GetDefaultForOrg(ctx, orgID); err == nil && ac != nil {
		return &ac.ID
	}
	return nil
}
