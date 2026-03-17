//go:build desktop

package app

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"arkloop/services/shared/database/sqliteadapter"
	"arkloop/services/shared/database/sqlitepgx"
	"arkloop/services/shared/eventbus"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/executor"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/personas"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/subagentctl"
	"arkloop/services/worker/internal/tools"
	"arkloop/services/worker/internal/tools/builtin"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestDesktopSubAgentSchemaAvailable(t *testing.T) {
	ctx := context.Background()
	db, err := sqlitepgx.Open(filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if desktopSubAgentSchemaAvailable(ctx, db) {
		t.Fatal("expected sub-agent schema to be absent")
	}

	for _, stmt := range []string{
		`CREATE TABLE sub_agents (id TEXT PRIMARY KEY)`,
		`CREATE TABLE sub_agent_events (id TEXT PRIMARY KEY)`,
		`CREATE TABLE sub_agent_pending_inputs (id TEXT PRIMARY KEY)`,
		`CREATE TABLE sub_agent_context_snapshots (id TEXT PRIMARY KEY)`,
	} {
		if _, err := db.Exec(ctx, stmt); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	if !desktopSubAgentSchemaAvailable(ctx, db) {
		t.Fatal("expected sub-agent schema to be detected")
	}
}

type desktopNoopSubAgentControl struct{}

func (desktopNoopSubAgentControl) Spawn(context.Context, subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{SubAgentID: uuid.New()}, nil
}
func (desktopNoopSubAgentControl) SendInput(context.Context, subagentctl.SendInputRequest) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{}, nil
}
func (desktopNoopSubAgentControl) Wait(context.Context, subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{}, nil
}
func (desktopNoopSubAgentControl) Resume(context.Context, subagentctl.ResumeRequest) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{}, nil
}
func (desktopNoopSubAgentControl) Close(context.Context, subagentctl.CloseRequest) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{}, nil
}
func (desktopNoopSubAgentControl) Interrupt(context.Context, subagentctl.InterruptRequest) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{}, nil
}
func (desktopNoopSubAgentControl) GetStatus(context.Context, uuid.UUID) (subagentctl.StatusSnapshot, error) {
	return subagentctl.StatusSnapshot{}, nil
}
func (desktopNoopSubAgentControl) ListChildren(context.Context) ([]subagentctl.StatusSnapshot, error) {
	return nil, nil
}

func TestDesktopNormalPersonaSearchableIncludesSpawnAgent(t *testing.T) {
	registry := tools.NewRegistry()
	for _, spec := range builtin.AgentSpecs() {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register builtin tool: %v", err)
		}
	}

	executors := builtin.Executors(nil, nil, nil)
	allowlist := map[string]struct{}{}
	for _, name := range registry.ListNames() {
		if executors[name] != nil {
			allowlist[name] = struct{}{}
		}
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	personaDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "personas")
	personaRegistry, err := personas.LoadRegistry(personaDir)
	if err != nil {
		t.Fatalf("load personas: %v", err)
	}
	def, ok := personaRegistry.Get("normal")
	if !ok {
		t.Fatal("normal persona not found")
	}

	rc := &pipeline.RunContext{
		Run:               dataRunForDesktopTest(),
		Emitter:           events.NewEmitter("test"),
		ToolRegistry:      registry,
		ToolExecutors:     pipeline.CopyToolExecutors(executors),
		ToolSpecs:         append([]llm.ToolSpec{}, builtin.LlmSpecs()...),
		AllowlistSet:      pipeline.CopyStringSet(allowlist),
		PersonaDefinition: &def,
		SubAgentControl:   desktopNoopSubAgentControl{},
	}

	handler := pipeline.Build([]pipeline.RunMiddleware{
		pipeline.NewSpawnAgentMiddleware(),
		pipeline.NewToolBuildMiddleware(),
	}, func(_ context.Context, _ *pipeline.RunContext) error { return nil })
	if err := handler(context.Background(), rc); err != nil {
		t.Fatalf("build pipeline: %v", err)
	}

	searchable := rc.ToolExecutor.SearchableSpecs()
	if _, ok := searchable["spawn_agent"]; !ok {
		t.Fatalf("spawn_agent missing from searchable specs: %v", mapKeys(searchable))
	}
}

func TestComposeDesktopEngineRegistersArtifactTools(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	t.Setenv("ARKLOOP_DATA_DIR", dataDir)

	db, err := sqlitepgx.Open(filepath.Join(dataDir, "desktop.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	engine, err := ComposeDesktopEngine(ctx, db, eventbus.NewLocalEventBus(), executor.DefaultExecutorRegistry(), nil)
	if err != nil {
		t.Fatalf("compose desktop engine: %v", err)
	}

	for _, toolName := range []string{"artifact_guidelines", "show_widget", "create_artifact", "document_write"} {
		if _, ok := engine.toolRegistry.Get(toolName); !ok {
			t.Fatalf("expected tool %s to be registered", toolName)
		}
		if _, ok := engine.baseAllowlist[toolName]; !ok {
			t.Fatalf("expected tool %s in desktop allowlist", toolName)
		}
	}

	specNames := map[string]struct{}{}
	for _, spec := range engine.allLlmSpecs {
		specNames[spec.Name] = struct{}{}
	}
	for _, toolName := range []string{"artifact_guidelines", "show_widget", "create_artifact", "document_write"} {
		if _, ok := specNames[toolName]; !ok {
			t.Fatalf("expected tool spec %s in desktop llm specs", toolName)
		}
	}
}

func TestDesktopEventWriterCommitsNonStreamingEventsBeforeToolExecution(t *testing.T) {
	ctx := context.Background()

	sqlitePool, err := sqliteadapter.AutoMigrate(ctx, filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}
	defer sqlitePool.Close()

	db := sqlitepgx.New(sqlitePool.Unwrap())

	accountID := uuid.New()
	projectID := uuid.New()
	threadID := uuid.New()
	runID := uuid.New()

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{
			sql:  `INSERT INTO accounts (id, slug, name, type, status) VALUES ($1, $2, $3, 'personal', 'active')`,
			args: []any{accountID, "desktop-writer-test-" + accountID.String(), "Desktop Writer Test"},
		},
		{
			sql:  `INSERT INTO projects (id, account_id, name, visibility) VALUES ($1, $2, $3, 'private')`,
			args: []any{projectID, accountID, "Writer Project"},
		},
		{
			sql:  `INSERT INTO threads (id, account_id, project_id, is_private) VALUES ($1, $2, $3, TRUE)`,
			args: []any{threadID, accountID, projectID},
		},
		{
			sql:  `INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running')`,
			args: []any{runID, accountID, threadID},
		},
	} {
		if _, err := db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed data: %v", err)
		}
	}

	writer := &desktopEventWriter{
		db:         db,
		run:        data.Run{ID: runID, AccountID: accountID, ThreadID: threadID},
		traceID:    "test-trace",
		runsRepo:   data.DesktopRunsRepository{},
		eventsRepo: data.DesktopRunEventsRepository{},
	}

	completedTurn := events.RunEvent{
		Type: "llm.turn.completed",
		DataJSON: map[string]any{
			"usage": map[string]any{
				"input_tokens":  12,
				"output_tokens": 7,
			},
		},
	}
	if err := writer.append(ctx, runID, completedTurn, "normal"); err != nil {
		t.Fatalf("append non-streaming event: %v", err)
	}
	if writer.tx != nil {
		t.Fatal("expected non-streaming event to commit writer transaction")
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin sub-agent tx: %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := (data.SubAgentRepository{}).Create(ctx, tx, data.SubAgentCreateParams{
		AccountID:      accountID,
		ParentRunID:    runID,
		ParentThreadID: threadID,
		RootRunID:      runID,
		RootThreadID:   threadID,
		Depth:          1,
		SourceType:     data.SubAgentSourceTypeThreadSpawn,
		ContextMode:    data.SubAgentContextModeIsolated,
	}); err != nil {
		t.Fatalf("create sub_agent after non-streaming commit: %v", err)
	}
}

func TestDesktopSubAgentContextRestoresRoutingFromSnapshotFallback(t *testing.T) {
	ctx := context.Background()

	sqlitePool, err := sqliteadapter.AutoMigrate(ctx, filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}
	defer sqlitePool.Close()

	db := sqlitepgx.New(sqlitePool.Unwrap())

	accountID := uuid.New()
	projectID := uuid.New()
	parentThreadID := uuid.New()
	childThreadID := uuid.New()
	parentRunID := uuid.New()
	childRunID := uuid.New()
	subAgentID := uuid.New()

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{
			sql:  `INSERT INTO accounts (id, slug, name, type, status) VALUES ($1, $2, $3, 'personal', 'active')`,
			args: []any{accountID, "desktop-subagent-routing-" + accountID.String(), "Desktop SubAgent Routing"},
		},
		{
			sql:  `INSERT INTO projects (id, account_id, name, visibility) VALUES ($1, $2, $3, 'private')`,
			args: []any{projectID, accountID, "Routing Project"},
		},
		{
			sql:  `INSERT INTO threads (id, account_id, project_id, is_private) VALUES ($1, $2, $3, TRUE), ($4, $2, $3, TRUE)`,
			args: []any{parentThreadID, accountID, projectID, childThreadID},
		},
		{
			sql:  `INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running'), ($4, $2, $5, 'running')`,
			args: []any{parentRunID, accountID, parentThreadID, childRunID, childThreadID},
		},
		{
			sql: `INSERT INTO sub_agents
				(id, account_id, parent_run_id, parent_thread_id, root_run_id, root_thread_id, depth, source_type, context_mode, status, current_run_id)
				VALUES ($1, $2, $3, $4, $3, $4, 1, $5, $6, $7, $8)`,
			args: []any{subAgentID, accountID, parentRunID, parentThreadID, data.SubAgentSourceTypeThreadSpawn, data.SubAgentContextModeIsolated, data.SubAgentStatusQueued, childRunID},
		},
	} {
		if _, err := db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed data: %v", err)
		}
	}

	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin snapshot tx: %v", err)
	}
	storage := subagentctl.NewSnapshotStorage()
	if err := storage.Save(ctx, tx, subAgentID, subagentctl.ContextSnapshot{
		ContextMode: data.SubAgentContextModeIsolated,
		Routing: &subagentctl.ContextSnapshotRouting{
			RouteID: "route-parent",
			Model:   "anthropic^claude-sonnet-4-5",
		},
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit snapshot: %v", err)
	}

	rc := &pipeline.RunContext{
		Run:       data.Run{ID: childRunID, AccountID: accountID, ThreadID: childThreadID, ParentRunID: &parentRunID},
		InputJSON: map[string]any{},
	}

	mw := desktopSubAgentContext(db, storage)
	if err := mw(ctx, rc, func(_ context.Context, rc *pipeline.RunContext) error {
		if got := rc.InputJSON["route_id"]; got != "route-parent" {
			t.Fatalf("unexpected route_id: %#v", got)
		}
		if got := rc.InputJSON["model"]; got != "anthropic^claude-sonnet-4-5" {
			t.Fatalf("unexpected model: %#v", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("middleware failed: %v", err)
	}
}

func TestDesktopEventWriterTouchesRunActivityOnNonTerminalCommit(t *testing.T) {
	ctx := context.Background()

	sqlitePool, err := sqliteadapter.AutoMigrate(ctx, filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}
	defer sqlitePool.Close()

	db := sqlitepgx.New(sqlitePool.Unwrap())

	accountID := uuid.New()
	projectID := uuid.New()
	threadID := uuid.New()
	runID := uuid.New()

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{
			sql:  `INSERT INTO accounts (id, slug, name, type, status) VALUES ($1, $2, $3, 'personal', 'active')`,
			args: []any{accountID, "desktop-activity-test-" + accountID.String(), "Desktop Activity Test"},
		},
		{
			sql:  `INSERT INTO projects (id, account_id, name, visibility) VALUES ($1, $2, $3, 'private')`,
			args: []any{projectID, accountID, "Activity Project"},
		},
		{
			sql:  `INSERT INTO threads (id, account_id, project_id, is_private) VALUES ($1, $2, $3, TRUE)`,
			args: []any{threadID, accountID, projectID},
		},
		{
			sql:  `INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running')`,
			args: []any{runID, accountID, threadID},
		},
	} {
		if _, err := db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed data: %v", err)
		}
	}

	oldActivity := time.Date(2000, time.January, 2, 3, 4, 5, 0, time.UTC).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(ctx, `UPDATE runs SET status_updated_at = $2 WHERE id = $1`, runID, oldActivity); err != nil {
		t.Fatalf("set old activity: %v", err)
	}

	writer := &desktopEventWriter{
		db:         db,
		run:        data.Run{ID: runID, AccountID: accountID, ThreadID: threadID},
		traceID:    "desktop-activity-trace",
		runsRepo:   data.DesktopRunsRepository{},
		eventsRepo: data.DesktopRunEventsRepository{},
	}

	ev := events.RunEvent{
		Type: "llm.turn.completed",
		DataJSON: map[string]any{
			"usage": map[string]any{
				"input_tokens":  5,
				"output_tokens": 4,
			},
		},
	}
	if err := writer.append(ctx, runID, ev, "normal"); err != nil {
		t.Fatalf("append non-terminal event: %v", err)
	}
	if err := writer.flush(ctx); err != nil {
		t.Fatalf("flush writer: %v", err)
	}

	var (
		status  string
		touched int
	)
	if err := db.QueryRow(
		ctx,
		`SELECT status,
		        CASE WHEN status_updated_at > $2 THEN 1 ELSE 0 END
		   FROM runs
		  WHERE id = $1`,
		runID,
		oldActivity,
	).Scan(&status, &touched); err != nil {
		t.Fatalf("query run activity: %v", err)
	}
	if status != "running" {
		t.Fatalf("expected run to stay running, got %q", status)
	}
	if touched != 1 {
		t.Fatal("expected status_updated_at to refresh on non-terminal commit")
	}
}

func TestDesktopChannelContextOverridesUserIDFromPayload(t *testing.T) {
	ctx := context.Background()

	sqlitePool, err := sqliteadapter.AutoMigrate(ctx, filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}
	defer sqlitePool.Close()

	db := sqlitepgx.New(sqlitePool.Unwrap())

	if _, err := db.Exec(
		ctx,
		`CREATE TABLE channel_identities (
			id TEXT PRIMARY KEY,
			channel_type TEXT NOT NULL,
			platform_subject_id TEXT NOT NULL,
			user_id TEXT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		)`,
	); err != nil {
		t.Fatalf("create channel_identities table: %v", err)
	}

	identityID := uuid.New()
	senderUserID := uuid.New()
	if _, err := db.Exec(
		ctx,
		`INSERT INTO channel_identities (id, channel_type, platform_subject_id, user_id, metadata_json)
		 VALUES ($1, 'telegram', '10001', $2, '{}')`,
		identityID,
		senderUserID,
	); err != nil {
		t.Fatalf("insert channel identity: %v", err)
	}

	originalUserID := uuid.New()
	channelID := uuid.New()
	rc := &pipeline.RunContext{
		UserID: &originalUserID,
		JobPayload: map[string]any{
			"channel_delivery": map[string]any{
				"channel_id":                 channelID.String(),
				"channel_type":               "telegram",
				"platform_chat_id":           "10001",
				"sender_channel_identity_id": identityID.String(),
			},
		},
	}

	mw := desktopChannelContext(db)
	if err := mw(ctx, rc, func(_ context.Context, rc *pipeline.RunContext) error {
		if rc.ChannelContext == nil {
			t.Fatal("expected channel context to be populated")
		}
		if rc.UserID == nil || *rc.UserID != senderUserID {
			t.Fatalf("expected user override to sender user, got %#v", rc.UserID)
		}
		if rc.ChannelContext.SenderUserID == nil || *rc.ChannelContext.SenderUserID != senderUserID {
			t.Fatalf("unexpected sender user id: %#v", rc.ChannelContext.SenderUserID)
		}
		return nil
	}); err != nil {
		t.Fatalf("desktop channel context failed: %v", err)
	}
}

func TestDesktopChannelDeliveryRecordsFailureWhenChannelMissing(t *testing.T) {
	ctx := context.Background()

	sqlitePool, err := sqliteadapter.AutoMigrate(ctx, filepath.Join(t.TempDir(), "desktop.db"))
	if err != nil {
		t.Fatalf("auto migrate sqlite: %v", err)
	}
	defer sqlitePool.Close()

	db := sqlitepgx.New(sqlitePool.Unwrap())

	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS channels (
			id TEXT PRIMARY KEY,
			channel_type TEXT NOT NULL,
			credentials_id TEXT NULL,
			is_active INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			id TEXT PRIMARY KEY,
			encrypted_value TEXT NULL,
			key_version INTEGER NULL
		)`,
	} {
		if _, err := db.Exec(ctx, stmt); err != nil {
			t.Fatalf("create channel tables: %v", err)
		}
	}

	accountID := uuid.New()
	projectID := uuid.New()
	threadID := uuid.New()
	runID := uuid.New()

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{
			sql:  `INSERT INTO accounts (id, slug, name, type, status) VALUES ($1, $2, $3, 'personal', 'active')`,
			args: []any{accountID, "desktop-channel-test-" + accountID.String(), "Desktop Channel Test"},
		},
		{
			sql:  `INSERT INTO projects (id, account_id, name, visibility) VALUES ($1, $2, $3, 'private')`,
			args: []any{projectID, accountID, "Channel Project"},
		},
		{
			sql:  `INSERT INTO threads (id, account_id, project_id, is_private) VALUES ($1, $2, $3, TRUE)`,
			args: []any{threadID, accountID, projectID},
		},
		{
			sql:  `INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running')`,
			args: []any{runID, accountID, threadID},
		},
	} {
		if _, err := db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed data: %v", err)
		}
	}

	rc := &pipeline.RunContext{
		Run:                  data.Run{ID: runID, AccountID: accountID, ThreadID: threadID},
		FinalAssistantOutput: "你好，来自 desktop。",
		ChannelContext: &pipeline.ChannelContext{
			ChannelID:      uuid.New(),
			ChannelType:    "telegram",
			PlatformChatID: "10001",
		},
	}

	mw := desktopChannelDelivery(db)
	if err := mw(ctx, rc, func(_ context.Context, _ *pipeline.RunContext) error { return nil }); err != nil {
		t.Fatalf("desktop channel delivery middleware failed: %v", err)
	}

	var errorMessage string
	if err := db.QueryRow(
		ctx,
		`SELECT json_extract(data_json, '$.error')
		   FROM run_events
		  WHERE run_id = $1
		    AND type = 'run.channel_delivery_failed'
		  ORDER BY seq DESC
		  LIMIT 1`,
		runID,
	).Scan(&errorMessage); err != nil {
		t.Fatalf("load delivery failure event: %v", err)
	}
	if errorMessage != "channel not found or inactive" {
		t.Fatalf("unexpected delivery failure error: %q", errorMessage)
	}
}

func dataRunForDesktopTest() data.Run {
	return data.Run{
		ID:        uuid.New(),
		AccountID: uuid.New(),
		ThreadID:  uuid.New(),
	}
}

func mapKeys[K comparable, V any](items map[K]V) []K {
	keys := make([]K, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}
