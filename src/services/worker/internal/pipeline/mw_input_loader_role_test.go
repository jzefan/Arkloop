//go:build !desktop

package pipeline

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadRunInputsIncludesRoleFromFirstEvent(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "pipeline_input_loader_role")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	accountID := uuid.New()
	projectID := uuid.New()
	threadID := uuid.New()
	runID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO threads (id, account_id, project_id) VALUES ($1, $2, $3)`, threadID, accountID, projectID); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running')`, runID, accountID, threadID); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO run_events (run_id, seq, type, data_json) VALUES ($1, 1, 'run.started', '{"persona_id":"p1","role":"worker"}'::jsonb)`, runID); err != nil {
		t.Fatalf("insert run event: %v", err)
	}

	loaded, err := loadRunInputs(context.Background(), pool, data.Run{ID: runID, AccountID: accountID, ThreadID: threadID}, data.RunsRepository{}, data.RunEventsRepository{}, data.MessagesRepository{}, nil, nil, 20)
	if err != nil {
		t.Fatalf("loadRunInputs failed: %v", err)
	}
	if len(loaded.Messages) != 0 {
		t.Fatalf("expected no messages, got %d", len(loaded.Messages))
	}
	if got := loaded.InputJSON["role"]; got != "worker" {
		t.Fatalf("unexpected role: %#v", got)
	}
	if got := loaded.InputJSON["persona_id"]; got != "p1" {
		t.Fatalf("unexpected persona_id: %#v", got)
	}
}

func TestLoadRunInputsReplaysInterruptedRunBeforeTrailingUserInput(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupPostgresDatabase(t, "pipeline_input_loader_resume")
	pool, err := pgxpool.New(ctx, db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	accountID := uuid.New()
	projectID := uuid.New()
	threadID := uuid.New()
	parentRunID := uuid.New()
	resumedRunID := uuid.New()
	firstMessageID := uuid.New()
	continueMessageID := uuid.New()

	if _, err := pool.Exec(ctx, `INSERT INTO threads (id, account_id, project_id) VALUES ($1, $2, $3)`, threadID, accountID, projectID); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'interrupted')`, parentRunID, accountID, threadID); err != nil {
		t.Fatalf("insert parent run: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runs (id, account_id, thread_id, status, resume_from_run_id) VALUES ($1, $2, $3, 'running', $4)`, resumedRunID, accountID, threadID, parentRunID); err != nil {
		t.Fatalf("insert resumed run: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO run_events (run_id, seq, type, data_json) VALUES ($1, 1, 'run.started', $2::jsonb)`, parentRunID, `{"thread_tail_message_id":"`+firstMessageID.String()+`"}`); err != nil {
		t.Fatalf("insert parent run started event: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO run_events (run_id, seq, type, data_json) VALUES ($1, 1, 'run.started', '{}'::jsonb)`, resumedRunID); err != nil {
		t.Fatalf("insert resumed run started event: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (id, account_id, thread_id, role, content, metadata_json, hidden) VALUES ($1, $2, $3, 'user', $4, '{}'::jsonb, false)`, firstMessageID, accountID, threadID, "find the file"); err != nil {
		t.Fatalf("insert first user message: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO messages (id, account_id, thread_id, role, content, metadata_json, hidden) VALUES ($1, $2, $3, 'user', $4, '{}'::jsonb, false)`, continueMessageID, accountID, threadID, "continue"); err != nil {
		t.Fatalf("insert trailing user message: %v", err)
	}

	storeRoot := t.TempDir()
	store, err := objectstore.NewFilesystemOpener(storeRoot).Open(ctx, objectstore.RolloutBucket)
	if err != nil {
		t.Fatalf("open rollout store: %v", err)
	}
	blobStore, ok := store.(objectstore.BlobStore)
	if !ok {
		t.Fatal("expected blob store")
	}

	toolCallsJSON, err := json.Marshal([]llm.ToolCall{{
		ToolCallID:    "call_1",
		ToolName:      "echo",
		ArgumentsJSON: map[string]any{"text": "hi"},
	}})
	if err != nil {
		t.Fatalf("marshal tool calls: %v", err)
	}
	toolInputJSON, err := json.Marshal(map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}
	rolloutBody := marshalRolloutJSONL(t,
		map[string]any{"type": "turn_start", "payload": map[string]any{"turn_index": 1, "model": "stub"}},
		map[string]any{"type": "assistant_message", "payload": map[string]any{"content": "I am checking", "tool_calls": json.RawMessage(toolCallsJSON)}},
		map[string]any{"type": "tool_call", "payload": map[string]any{"call_id": "call_1", "name": "echo", "input": json.RawMessage(toolInputJSON)}},
		map[string]any{"type": "run_end", "payload": map[string]any{"final_status": "interrupted"}},
	)
	if err := blobStore.Put(ctx, "run/"+parentRunID.String()+".jsonl", rolloutBody); err != nil {
		t.Fatalf("write rollout: %v", err)
	}

	loaded, err := loadRunInputs(ctx, pool, data.Run{
		ID:              resumedRunID,
		AccountID:       accountID,
		ThreadID:        threadID,
		ResumeFromRunID: &parentRunID,
	}, data.RunsRepository{}, data.RunEventsRepository{}, data.MessagesRepository{}, nil, blobStore, 20)
	if err != nil {
		t.Fatalf("loadRunInputs failed: %v", err)
	}
	if got := loaded.InputJSON["last_user_message"]; got != "continue" {
		t.Fatalf("unexpected last_user_message: %#v", got)
	}
	if len(loaded.Messages) != 4 {
		t.Fatalf("expected 4 prompt messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != "user" || loaded.Messages[0].Content[0].Text != "find the file" {
		t.Fatalf("unexpected first message: %#v", loaded.Messages[0])
	}
	if loaded.Messages[1].Role != "assistant" || loaded.Messages[1].Content[0].Text != "I am checking" {
		t.Fatalf("unexpected replayed assistant message: %#v", loaded.Messages[1])
	}
	if len(loaded.Messages[1].ToolCalls) != 1 || loaded.Messages[1].ToolCalls[0].ToolCallID != "call_1" {
		t.Fatalf("unexpected replayed tool calls: %#v", loaded.Messages[1].ToolCalls)
	}
	if loaded.Messages[2].Role != "tool" {
		t.Fatalf("unexpected replayed tool result role: %#v", loaded.Messages[2])
	}
	var toolEnvelope map[string]any
	if err := json.Unmarshal([]byte(loaded.Messages[2].Content[0].Text), &toolEnvelope); err != nil {
		t.Fatalf("decode replayed tool result: %v", err)
	}
	if toolEnvelope["tool_call_id"] != "call_1" {
		t.Fatalf("unexpected replayed tool_call_id: %#v", toolEnvelope["tool_call_id"])
	}
	errorPayload, ok := toolEnvelope["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected synthetic tool error payload, got %#v", toolEnvelope["error"])
	}
	if errorPayload["error_class"] != interruptedToolErrorClass {
		t.Fatalf("unexpected synthetic tool error class: %#v", errorPayload["error_class"])
	}
	if loaded.Messages[3].Role != "user" || loaded.Messages[3].Content[0].Text != "continue" {
		t.Fatalf("unexpected trailing user message: %#v", loaded.Messages[3])
	}
	if len(loaded.ThreadMessageIDs) != 4 {
		t.Fatalf("expected 4 thread message ids, got %d", len(loaded.ThreadMessageIDs))
	}
	if loaded.ThreadMessageIDs[1] != uuid.Nil || loaded.ThreadMessageIDs[2] != uuid.Nil {
		t.Fatalf("expected replayed entries to use nil ids, got %#v", loaded.ThreadMessageIDs)
	}
	if loaded.ThreadMessageIDs[0] != firstMessageID || loaded.ThreadMessageIDs[3] != continueMessageID {
		t.Fatalf("unexpected preserved thread message ids: %#v", loaded.ThreadMessageIDs)
	}
}

func marshalRolloutJSONL(t *testing.T, items ...map[string]any) []byte {
	t.Helper()
	var out []byte
	for _, item := range items {
		item["timestamp"] = time.Now().UTC()
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal rollout item: %v", err)
		}
		out = append(out, encoded...)
		out = append(out, '\n')
	}
	return out
}
