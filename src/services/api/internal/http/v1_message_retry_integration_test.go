//go:build !desktop

package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	nethttp "net/http"

	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type messageRetryTestEnv struct {
	ctx         context.Context
	pool        *pgxpool.Pool
	handler     nethttp.Handler
	headers     map[string]string
	accountID   uuid.UUID
	userID      uuid.UUID
	messageRepo *data.MessageRepository
	runRepo     *data.RunEventRepository
}

func setupMessageRetryTestEnv(t *testing.T, prefix string) *messageRetryTestEnv {
	t.Helper()

	db := setupTestDatabase(t, prefix)
	ctx := context.Background()
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 32, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	passwordHasher, err := auth.NewBcryptPasswordHasher(0)
	if err != nil {
		t.Fatalf("new password hasher: %v", err)
	}
	tokenService, err := auth.NewJwtAccessTokenService("test-secret-should-be-long-enough-32chars", 3600, 2592000)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	userRepo, _ := data.NewUserRepository(pool)
	credentialRepo, _ := data.NewUserCredentialRepository(pool)
	membershipRepo, _ := data.NewAccountMembershipRepository(pool)
	refreshTokenRepo, _ := data.NewRefreshTokenRepository(pool)
	auditRepo, _ := data.NewAuditLogRepository(pool)
	threadRepo, _ := data.NewThreadRepository(pool)
	projectRepo, _ := data.NewProjectRepository(pool)
	messageRepo, _ := data.NewMessageRepository(pool)
	runRepo, _ := data.NewRunEventRepository(pool)
	jobRepo, _ := data.NewJobRepository(pool)

	authService, err := auth.NewService(userRepo, credentialRepo, membershipRepo, passwordHasher, tokenService, refreshTokenRepo, nil, nil)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	registrationService, err := auth.NewRegistrationService(pool, passwordHasher, tokenService, refreshTokenRepo, jobRepo)
	if err != nil {
		t.Fatalf("new registration service: %v", err)
	}
	auditWriter := audit.NewWriter(auditRepo, membershipRepo, logger)

	handler := NewHandler(HandlerConfig{
		Pool:                  pool,
		Logger:                logger,
		AuthService:           authService,
		RegistrationService:   registrationService,
		AccountMembershipRepo: membershipRepo,
		ThreadRepo:            threadRepo,
		ProjectRepo:           projectRepo,
		MessageRepo:           messageRepo,
		RunEventRepo:          runRepo,
		AuditWriter:           auditWriter,
		TrustIncomingTraceID:  true,
	})

	registerResp := doJSON(
		handler,
		nethttp.MethodPost,
		"/v1/auth/register",
		map[string]any{"login": prefix + "-alice", "password": "pwd12345", "email": prefix + "-alice@test.com"},
		nil,
	)
	if registerResp.Code != nethttp.StatusCreated {
		t.Fatalf("register: %d %s", registerResp.Code, registerResp.Body.String())
	}
	alice := decodeJSONBody[registerResponse](t, registerResp.Body.Bytes())

	return &messageRetryTestEnv{
		ctx:         ctx,
		pool:        pool,
		handler:     handler,
		headers:     authHeader(alice.AccessToken),
		userID:      uuid.MustParse(alice.UserID),
		messageRepo: messageRepo,
		runRepo:     runRepo,
	}
}

func (e *messageRetryTestEnv) createThread(t *testing.T, title string) data.Thread {
	t.Helper()
	resp := doJSON(e.handler, nethttp.MethodPost, "/v1/threads", map[string]any{"title": title}, e.headers)
	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("create thread: %d %s", resp.Code, resp.Body.String())
	}
	payload := decodeJSONBody[threadResponse](t, resp.Body.Bytes())
	e.accountID = uuid.MustParse(payload.AccountID)
	return data.Thread{
		ID:        uuid.MustParse(payload.ID),
		AccountID: e.accountID,
	}
}

func (e *messageRetryTestEnv) createMessage(t *testing.T, thread data.Thread, role string, content string) data.Message {
	t.Helper()
	var createdBy *uuid.UUID
	if role == "user" {
		createdBy = &e.userID
	}
	msg, err := e.messageRepo.CreateStructured(e.ctx, thread.AccountID, thread.ID, role, content, nil, createdBy)
	if err != nil {
		t.Fatalf("create %s message: %v", role, err)
	}
	return msg
}

func (e *messageRetryTestEnv) createCompletedRun(t *testing.T, thread data.Thread, startedData map[string]any) data.Run {
	t.Helper()
	run, _, err := e.runRepo.CreateRootRunWithClaim(e.ctx, thread.AccountID, thread.ID, &e.userID, "run.started", startedData)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := e.pool.Exec(e.ctx, `UPDATE runs SET status = 'completed', completed_at = NOW(), status_updated_at = NOW() WHERE id = $1`, run.ID); err != nil {
		t.Fatalf("complete run: %v", err)
	}
	run.Status = "completed"
	return run
}

func (e *messageRetryTestEnv) readRunStartedData(t *testing.T, runID uuid.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := e.pool.QueryRow(e.ctx, `SELECT data_json FROM run_events WHERE run_id = $1 AND type = 'run.started' LIMIT 1`, runID).Scan(&raw); err != nil {
		t.Fatalf("read run.started: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode run.started: %v raw=%s", err, string(raw))
	}
	return out
}

func (e *messageRetryTestEnv) retryMessage(t *testing.T, threadID uuid.UUID, messageID uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(e.handler, nethttp.MethodPost, "/v1/threads/"+threadID.String()+"/messages/"+messageID.String()+":retry", body, e.headers)
}

func TestMessageRetryTailUserReusesThreadWithoutAppendingMessage(t *testing.T) {
	env := setupMessageRetryTestEnv(t, "api_go_message_retry_tail")
	thread := env.createThread(t, "retry-tail")
	userMsg := env.createMessage(t, thread, "user", "hello")

	resp := env.retryMessage(t, thread.ID, userMsg.ID, map[string]any{})
	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("retry message: %d %s", resp.Code, resp.Body.String())
	}
	runPayload := decodeJSONBody[createRunResponse](t, resp.Body.Bytes())
	startedData := env.readRunStartedData(t, uuid.MustParse(runPayload.RunID))
	if startedData["source"] != "retry" {
		t.Fatalf("unexpected source: %#v", startedData["source"])
	}
	if startedData["thread_tail_message_id"] != userMsg.ID.String() {
		t.Fatalf("unexpected tail message: %#v", startedData["thread_tail_message_id"])
	}

	listResp := doJSON(env.handler, nethttp.MethodGet, "/v1/threads/"+thread.ID.String()+"/messages", nil, env.headers)
	if listResp.Code != nethttp.StatusOK {
		t.Fatalf("list messages: %d %s", listResp.Code, listResp.Body.String())
	}
	messages := decodeJSONBody[[]messageResponse](t, listResp.Body.Bytes())
	if len(messages) != 1 || messages[0].ID != userMsg.ID.String() {
		t.Fatalf("retry should keep exactly the original user message, got %#v", messages)
	}
}

func TestMessageRetryMiddleUserCutsThreadAndInheritsRunData(t *testing.T) {
	env := setupMessageRetryTestEnv(t, "api_go_message_retry_middle")
	thread := env.createThread(t, "retry-middle")
	userA := env.createMessage(t, thread, "user", "A")
	env.createCompletedRun(t, thread, map[string]any{"model": "model-a", "persona_id": "persona-a"})
	assistantA := env.createMessage(t, thread, "assistant", "answer A")
	userB := env.createMessage(t, thread, "user", "B")
	env.createCompletedRun(t, thread, map[string]any{"model": "model-b", "persona_id": "persona-b"})
	assistantB := env.createMessage(t, thread, "assistant", "answer B")
	userC := env.createMessage(t, thread, "user", "C")

	resp := env.retryMessage(t, thread.ID, userB.ID, map[string]any{})
	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("retry message: %d %s", resp.Code, resp.Body.String())
	}
	runPayload := decodeJSONBody[createRunResponse](t, resp.Body.Bytes())
	startedData := env.readRunStartedData(t, uuid.MustParse(runPayload.RunID))
	if startedData["source"] != "retry" || startedData["thread_tail_message_id"] != userB.ID.String() {
		t.Fatalf("unexpected retry started data: %#v", startedData)
	}
	if startedData["model"] != "model-b" || startedData["persona_id"] != "persona-b" {
		t.Fatalf("retry did not inherit target run data: %#v", startedData)
	}

	listResp := doJSON(env.handler, nethttp.MethodGet, "/v1/threads/"+thread.ID.String()+"/messages", nil, env.headers)
	if listResp.Code != nethttp.StatusOK {
		t.Fatalf("list messages: %d %s", listResp.Code, listResp.Body.String())
	}
	messages := decodeJSONBody[[]messageResponse](t, listResp.Body.Bytes())
	gotIDs := []string{}
	for _, message := range messages {
		gotIDs = append(gotIDs, message.ID)
	}
	wantIDs := []string{userA.ID.String(), assistantA.ID.String(), userB.ID.String()}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("unexpected visible messages: got=%v want=%v", gotIDs, wantIDs)
	}

	for _, hiddenMsg := range []data.Message{assistantB, userC} {
		var hidden bool
		var deleted bool
		if err := env.pool.QueryRow(env.ctx, `SELECT hidden, deleted_at IS NOT NULL FROM messages WHERE id = $1`, hiddenMsg.ID).Scan(&hidden, &deleted); err != nil {
			t.Fatalf("read hidden message: %v", err)
		}
		if !hidden || !deleted {
			t.Fatalf("expected %s to be hidden and deleted, hidden=%v deleted=%v", hiddenMsg.ID, hidden, deleted)
		}
	}
}

func TestMessageRetryRollsBackCutWhenThreadBusy(t *testing.T) {
	env := setupMessageRetryTestEnv(t, "api_go_message_retry_busy")
	thread := env.createThread(t, "retry-busy")
	userA := env.createMessage(t, thread, "user", "A")
	assistantA := env.createMessage(t, thread, "assistant", "answer A")
	userB := env.createMessage(t, thread, "user", "B")
	if _, _, err := env.runRepo.CreateRootRunWithClaim(env.ctx, thread.AccountID, thread.ID, &env.userID, "run.started", map[string]any{}); err != nil {
		t.Fatalf("create active run: %v", err)
	}

	resp := env.retryMessage(t, thread.ID, userA.ID, map[string]any{})
	assertErrorEnvelope(t, resp, nethttp.StatusConflict, "runs.thread_busy")

	listResp := doJSON(env.handler, nethttp.MethodGet, "/v1/threads/"+thread.ID.String()+"/messages", nil, env.headers)
	if listResp.Code != nethttp.StatusOK {
		t.Fatalf("list messages: %d %s", listResp.Code, listResp.Body.String())
	}
	messages := decodeJSONBody[[]messageResponse](t, listResp.Body.Bytes())
	gotIDs := []string{}
	for _, message := range messages {
		gotIDs = append(gotIDs, message.ID)
	}
	wantIDs := []string{userA.ID.String(), assistantA.ID.String(), userB.ID.String()}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Fatalf("busy retry should roll back hidden messages: got=%v want=%v", gotIDs, wantIDs)
	}
}
