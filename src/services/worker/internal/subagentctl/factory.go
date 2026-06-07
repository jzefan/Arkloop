package subagentctl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const childThreadTTL = 7 * 24 * time.Hour

type SubAgentRunFactory struct {
	pool            data.DB
	snapshotStorage *SnapshotStorage
}

func NewSubAgentRunFactory(pool data.DB, snapshotStorage *SnapshotStorage) *SubAgentRunFactory {
	return &SubAgentRunFactory{pool: pool, snapshotStorage: snapshotStorage}
}

func (f *SubAgentRunFactory) CreateSpawnRun(
	ctx context.Context,
	tx pgx.Tx,
	parentRun data.Run,
	spawnReq ResolvedSpawnRequest,
	snapshot ContextSnapshot,
	forcedRunID *uuid.UUID,
) (data.SubAgentRecord, uuid.UUID, error) {
	parentSubAgent, err := (data.SubAgentRepository{}).GetByCurrentRunID(ctx, tx, parentRun.ID)
	if err != nil {
		return data.SubAgentRecord{}, uuid.Nil, err
	}
	ownerThreadID := parentRun.ThreadID
	var parentSubAgentID *uuid.UUID
	depth := 1
	if parentSubAgent != nil {
		ownerThreadID = parentRun.ThreadID
		parentSubAgentID = &parentSubAgent.ID
		depth = parentSubAgent.Depth + 1
	}
	childThreadID, err := f.createChildThread(ctx, tx, parentRun)
	if err != nil {
		return data.SubAgentRecord{}, uuid.Nil, err
	}
	createdSubAgent, err := (data.SubAgentRepository{}).Create(ctx, tx, data.SubAgentCreateParams{
		AccountID:        parentRun.AccountID,
		OwnerThreadID:    ownerThreadID,
		AgentThreadID:    childThreadID,
		OriginRunID:      parentRun.ID,
		ParentSubAgentID: parentSubAgentID,
		Depth:            depth,
		Role:             cloneStringPtr(spawnReq.Role),
		PersonaID:        stringPtr(spawnReq.PersonaID),
		Nickname:         cloneStringPtr(spawnReq.Nickname),
		SourceType:       spawnReq.SourceType,
		ContextMode:      spawnReq.ContextMode,
	})
	if err != nil {
		return data.SubAgentRecord{}, uuid.Nil, fmt.Errorf("create sub_agent: %w", err)
	}
	if err := f.snapshotStorage.Save(ctx, tx, createdSubAgent.ID, snapshot); err != nil {
		return data.SubAgentRecord{}, uuid.Nil, fmt.Errorf("save context snapshot: %w", err)
	}
	if _, err := (data.SubAgentEventAppender{}).Append(ctx, tx, createdSubAgent.ID, nil, data.SubAgentEventTypeSpawnRequested, map[string]any{
		"persona_id":      spawnReq.PersonaID,
		"context_mode":    createdSubAgent.ContextMode,
		"source_type":     spawnReq.SourceType,
		"origin_run_id":   parentRun.ID.String(),
		"owner_thread_id": ownerThreadID.String(),
		"agent_thread_id": childThreadID.String(),
	}, nil); err != nil {
		return data.SubAgentRecord{}, uuid.Nil, fmt.Errorf("append spawn_requested: %w", err)
	}
	if err := f.copySnapshotMessages(ctx, tx, parentRun.AccountID, childThreadID, snapshot.Messages); err != nil {
		return data.SubAgentRecord{}, uuid.Nil, err
	}
	if _, err := insertUserMessage(ctx, tx, parentRun.AccountID, childThreadID, spawnReq.Input); err != nil {
		return data.SubAgentRecord{}, uuid.Nil, fmt.Errorf("insert child message: %w", err)
	}
	childRunID, err := f.createQueuedRun(ctx, tx, parentRun, createdSubAgent, childThreadID, &snapshot, forcedRunID, data.SubAgentEventTypeSpawned, map[string]any{
		"thread_id": childThreadID.String(),
	}, nil, spawnReq.Model)
	if err != nil {
		return data.SubAgentRecord{}, uuid.Nil, err
	}
	return createdSubAgent, childRunID, nil
}

func (f *SubAgentRunFactory) CreateRunForExistingSubAgent(
	ctx context.Context,
	tx pgx.Tx,
	subAgent data.SubAgentRecord,
	input string,
	forcedRunID *uuid.UUID,
	primaryEventType string,
	primaryPayload map[string]any,
	errorClass *string,
	reconstructedMessages []ContextSnapshotMessage,
) (uuid.UUID, error) {
	ownerRun, err := (data.RunsRepository{}).GetRun(ctx, tx, subAgent.OriginRunID)
	if err != nil {
		return uuid.Nil, err
	}
	if ownerRun == nil {
		return uuid.Nil, fmt.Errorf("origin run not found: %s", subAgent.OriginRunID)
	}
	snapshot, err := f.snapshotStorage.LoadBySubAgent(ctx, tx, subAgent.ID)
	if err != nil {
		return uuid.Nil, err
	}
	if snapshot == nil {
		return uuid.Nil, fmt.Errorf("context snapshot not found for sub_agent: %s", subAgent.ID)
	}
	threadID := subAgent.AgentThreadID
	runID := uuid.Nil
	if subAgent.CurrentRunID != nil {
		runID = *subAgent.CurrentRunID
	}
	payload := cloneMap(primaryPayload)
	payload["thread_id"] = threadID.String()
	if runID != uuid.Nil {
		payload["run_id"] = runID.String()
	}
	trimmedInput := strings.TrimSpace(input)
	if trimmedInput != "" {
		messageID, err := insertUserMessage(ctx, tx, subAgent.AccountID, threadID, trimmedInput)
		if err != nil {
			return uuid.Nil, fmt.Errorf("insert sub_agent input: %w", err)
		}
		payload["message_id"] = messageID.String()
		payload["input_bytes"] = len([]byte(trimmedInput))
	}
	// 注入从 rollout 重建的历史消息
	if len(reconstructedMessages) > 0 {
		if err := f.copySnapshotMessages(ctx, tx, subAgent.AccountID, threadID, reconstructedMessages); err != nil {
			return uuid.Nil, fmt.Errorf("copy reconstructed messages: %w", err)
		}
	}
	return f.createQueuedRun(ctx, tx, *ownerRun, subAgent, threadID, snapshot, forcedRunID, primaryEventType, payload, errorClass, "")
}

func (f *SubAgentRunFactory) CreateRunFromPendingInputs(ctx context.Context, tx pgx.Tx, subAgent data.SubAgentRecord) (*uuid.UUID, error) {
	pendingRepo := data.SubAgentPendingInputsRepository{}
	items, err := pendingRepo.ListBySubAgentForUpdate(ctx, tx, subAgent.ID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	snapshot, err := f.snapshotStorage.LoadBySubAgent(ctx, tx, subAgent.ID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("context snapshot not found for sub_agent: %s", subAgent.ID)
	}
	parts := make([]string, 0, len(items))
	ids := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		parts = append(parts, strings.TrimSpace(item.Input))
		ids = append(ids, item.ID)
	}
	combined := strings.Join(parts, "\n\n")
	ownerRun, err := (data.RunsRepository{}).GetRun(ctx, tx, subAgent.OriginRunID)
	if err != nil {
		return nil, err
	}
	if ownerRun == nil {
		return nil, fmt.Errorf("origin run not found: %s", subAgent.OriginRunID)
	}
	threadID := subAgent.AgentThreadID
	messageID, err := insertUserMessage(ctx, tx, subAgent.AccountID, threadID, combined)
	if err != nil {
		return nil, fmt.Errorf("insert pending input message: %w", err)
	}
	childRunID, err := f.createQueuedRun(ctx, tx, *ownerRun, subAgent, threadID, snapshot, nil, data.SubAgentEventTypeInputSent, map[string]any{
		"thread_id":     threadID.String(),
		"message_id":    messageID.String(),
		"input_bytes":   len([]byte(combined)),
		"pending_count": len(items),
		"from_pending":  true,
	}, nil, "")
	if err != nil {
		return nil, err
	}
	if err := pendingRepo.DeleteBatch(ctx, tx, ids); err != nil {
		return nil, err
	}
	return &childRunID, nil
}

func (f *SubAgentRunFactory) createChildThread(ctx context.Context, tx pgx.Tx, parentRun data.Run) (uuid.UUID, error) {
	if parentRun.ProjectID == nil {
		return uuid.Nil, fmt.Errorf("parent run project_id must not be nil")
	}
	expiresAt := time.Now().UTC().Add(childThreadTTL)
	var childThreadID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO threads (account_id, project_id, is_private, expires_at)
		 VALUES ($1, $2, TRUE, $3)
		 RETURNING id`,
		parentRun.AccountID,
		parentRun.ProjectID,
		expiresAt,
	).Scan(&childThreadID); err != nil {
		return uuid.Nil, fmt.Errorf("create child thread: %w", err)
	}
	return childThreadID, nil
}

func (f *SubAgentRunFactory) createQueuedRun(
	ctx context.Context,
	tx pgx.Tx,
	parentRun data.Run,
	subAgent data.SubAgentRecord,
	threadID uuid.UUID,
	snapshot *ContextSnapshot,
	forcedRunID *uuid.UUID,
	primaryEventType string,
	primaryPayload map[string]any,
	errorClass *string,
	modelOverride string,
) (uuid.UUID, error) {
	childRunID := uuid.New()
	if forcedRunID != nil && *forcedRunID != uuid.Nil {
		childRunID = *forcedRunID
	}
	profileRef, workspaceRef := inheritedBindings(parentRun, snapshot)

	createdByUserID := parentRun.CreatedByUserID
	if subAgent.SourceType == data.SubAgentSourceTypePlatformAgent {
		if sysUID, err := f.resolveSystemAgentUserID(ctx, tx); err == nil && sysUID != uuid.Nil {
			createdByUserID = &sysUID
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO runs (id, account_id, thread_id, parent_run_id, created_by_user_id, profile_ref, workspace_ref, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'running')`,
		childRunID,
		parentRun.AccountID,
		threadID,
		parentRun.ID,
		createdByUserID,
		profileRef,
		workspaceRef,
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert child run: %w", err)
	}
	// Lock the run row to serialize per-run seq allocation
	if _, err := tx.Exec(ctx, `SELECT 1 FROM runs WHERE id = $1 FOR UPDATE`, childRunID); err != nil {
		return uuid.Nil, fmt.Errorf("lock run for seq: %w", err)
	}
	var seq int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM run_events WHERE run_id = $1`,
		childRunID,
	).Scan(&seq); err != nil {
		return uuid.Nil, fmt.Errorf("alloc seq: %w", err)
	}
	personaID := derefString(subAgent.PersonaID)
	eventData, err := json.Marshal(buildRunStartedData(subAgent, snapshot, personaID, modelOverride))
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal run.started data: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO run_events (run_id, seq, type, data_json)
		 VALUES ($1, $2, 'run.started', $3::jsonb)`,
		childRunID, seq, string(eventData),
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert run.started: %w", err)
	}
	if err := (data.SubAgentRepository{}).TransitionToQueued(ctx, tx, subAgent.ID, childRunID); err != nil {
		return uuid.Nil, fmt.Errorf("mark sub_agent queued: %w", err)
	}
	payload := map[string]any{
		"run_id":       childRunID.String(),
		"thread_id":    threadID.String(),
		"persona_id":   personaID,
		"context_mode": subAgent.ContextMode,
	}
	for key, value := range primaryPayload {
		payload[key] = value
	}
	appender := data.SubAgentEventAppender{}
	if strings.TrimSpace(primaryEventType) != "" {
		if _, err := appender.Append(ctx, tx, subAgent.ID, &childRunID, primaryEventType, payload, errorClass); err != nil {
			return uuid.Nil, fmt.Errorf("append %s: %w", primaryEventType, err)
		}
	}
	if _, err := appender.Append(ctx, tx, subAgent.ID, &childRunID, data.SubAgentEventTypeRunQueued, payload, nil); err != nil {
		return uuid.Nil, fmt.Errorf("append run_queued: %w", err)
	}
	return childRunID, nil
}

func (f *SubAgentRunFactory) copySnapshotMessages(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, threadID uuid.UUID, messages []ContextSnapshotMessage) error {
	if len(messages) == 0 {
		return nil
	}
	// 过滤孤儿 tool 消息：OpenAI/兼容协议要求 role=tool 的消息必须紧随
	// （或不远地跟在）一条 assistant tool_calls 之后。父对话里如果存在
	// 没有匹配 tool_calls 前驱的 tool 消息（例如 context.emit("tool.result", …)
	// 落库但没有对应 assistant tool_calls，常见于通过 emit 直接发"虚拟"工具
	// 调用如进度 todo_write 的 persona），原样复制到子 thread 会让 VL/LLM
	// 拒收：`Messages with role 'tool' must be a response to a preceding
	// message with 'tool_calls'`。这里在复制前剔除所有这种孤儿。
	filtered := filterOrphanToolMessages(messages)
	repo := data.MessagesRepository{}
	for _, item := range filtered {
		hasContent := strings.TrimSpace(item.Content) != ""
		hasJSON := len(item.ContentJSON) > 0
		// 完全空消息（既无文本又无结构化负载）一律跳过——通常是上一次 run 在
		// 模型还没产出任何输出前就挂了，留下的占位 assistant 消息。
		if !hasContent && !hasJSON {
			continue
		}
		content := item.Content
		if !hasContent {
			// content_json 非空但 content 文本为空：messages_repo.InsertThreadMessage
			// 硬性要求 content trim 后非空（详见 messages_repo.go:738），直接传会让
			// 整个 spawn 失败。注入一个语义稳定的占位让插入通过，同时保留 content_json
			// 里的多模态/工具调用负载——下游 LLM 收到的还是完整结构化内容。
			content = "[non-text message]"
		}
		if _, err := repo.InsertThreadMessage(ctx, tx, accountID, threadID, item.Role, content, cloneRawJSON(item.ContentJSON), nil); err != nil {
			return fmt.Errorf("copy snapshot message: %w", err)
		}
	}
	return nil
}

// filterOrphanToolMessages 丢弃没有匹配 assistant tool_calls 前驱的 tool 消息。
//
// 判定规则：扫描时维护一个 "开放 tool_use id 集合"——遇到 assistant 消息时
// 解析它 content_json 里的 tool_use 块、把每个 id 加进集合；遇到 tool 消息时
// 看它的 tool_use_id 是否在集合里（在则放行并从集合移除，不在则丢弃）。
// 缺少 tool_use_id 字段的 tool 消息（典型如 context.emit("tool.result", …)
// 写出的 todo_write 进度结果）一律视为孤儿丢弃。
func filterOrphanToolMessages(messages []ContextSnapshotMessage) []ContextSnapshotMessage {
	openIDs := map[string]struct{}{}
	out := make([]ContextSnapshotMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ContentJSON) > 0 {
			var blocks []json.RawMessage
			if json.Unmarshal(msg.ContentJSON, &blocks) == nil {
				for _, raw := range blocks {
					var block map[string]any
					if json.Unmarshal(raw, &block) != nil {
						continue
					}
					if t, _ := block["type"].(string); t == "tool_use" {
						if id, _ := block["id"].(string); id != "" {
							openIDs[id] = struct{}{}
						}
					}
				}
			}
			out = append(out, msg)
			continue
		}
		if msg.Role == "tool" {
			useID := ""
			if len(msg.ContentJSON) > 0 {
				var toolMsg map[string]any
				if json.Unmarshal(msg.ContentJSON, &toolMsg) == nil {
					useID, _ = toolMsg["tool_use_id"].(string)
				}
			}
			if useID == "" {
				continue
			}
			if _, ok := openIDs[useID]; !ok {
				continue
			}
			delete(openIDs, useID)
			out = append(out, msg)
			continue
		}
		out = append(out, msg)
	}
	return out
}

func buildRunStartedData(subAgent data.SubAgentRecord, snapshot *ContextSnapshot, personaID string, modelOverride string) map[string]any {
	payload := map[string]any{
		"persona_id":   personaID,
		"sub_agent_id": subAgent.ID.String(),
		"context_mode": subAgent.ContextMode,
	}
	if subAgent.Role != nil && strings.TrimSpace(*subAgent.Role) != "" {
		payload["role"] = strings.TrimSpace(*subAgent.Role)
	}
	if model := strings.TrimSpace(modelOverride); model != "" {
		payload["model"] = model
	}
	if snapshot == nil {
		return payload
	}
	if routeID := strings.TrimSpace(snapshot.Runtime.RouteID); routeID != "" {
		payload["route_id"] = routeID
	}
	return payload
}

func inheritedBindings(parentRun data.Run, snapshot *ContextSnapshot) (*string, *string) {
	if snapshot == nil || !snapshot.Inherit.Workspace {
		return nil, nil
	}
	profileRef := strings.TrimSpace(snapshot.Environment.ProfileRef)
	workspaceRef := strings.TrimSpace(snapshot.Environment.WorkspaceRef)
	if profileRef == "" {
		profileRef = strings.TrimSpace(derefString(parentRun.ProfileRef))
	}
	if workspaceRef == "" {
		workspaceRef = strings.TrimSpace(derefString(parentRun.WorkspaceRef))
	}
	return stringPtr(profileRef), stringPtr(workspaceRef)
}

func resolveSubAgentThread(ctx context.Context, tx pgx.Tx, record data.SubAgentRecord) (uuid.UUID, uuid.UUID, error) {
	candidateRunID := uuid.Nil
	if record.CurrentRunID != nil {
		candidateRunID = *record.CurrentRunID
	} else if record.LastCompletedRunID != nil {
		candidateRunID = *record.LastCompletedRunID
	}
	if candidateRunID == uuid.Nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("sub_agent has no run context")
	}
	run, err := (data.RunsRepository{}).GetRun(ctx, tx, candidateRunID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if run == nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("run not found: %s", candidateRunID)
	}
	return run.ThreadID, run.ID, nil
}

func insertUserMessage(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, threadID uuid.UUID, content string) (uuid.UUID, error) {
	return data.MessagesRepository{}.InsertThreadMessage(ctx, tx, accountID, threadID, "user", strings.TrimSpace(content), nil, nil)
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

// resolveSystemAgentUserID 从 DB 查询 system_agent 的 user_id。
func (f *SubAgentRunFactory) resolveSystemAgentUserID(ctx context.Context, tx pgx.Tx) (uuid.UUID, error) {
	var uid uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM users WHERE username = 'system_agent' AND deleted_at IS NULL LIMIT 1`,
	).Scan(&uid)
	if err != nil {
		return uuid.Nil, err
	}
	return uid, nil
}
