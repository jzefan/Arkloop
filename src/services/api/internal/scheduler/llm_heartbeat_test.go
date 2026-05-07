package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func TestTriggerSchedulerFireOneEnqueuesChannelDeliveryPayloadForDiscordDM(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "scheduler_heartbeat_channel_delivery")
	ctx := context.Background()

	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 16, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	jobsRepo, err := data.NewJobRepository(pool)
	if err != nil {
		t.Fatalf("jobs repo: %v", err)
	}
	runRepo, err := data.NewRunEventRepository(pool)
	if err != nil {
		t.Fatalf("run repo: %v", err)
	}

	accountID := uuid.New()
	projectID := uuid.New()
	personaID := uuid.New()
	channelID := uuid.New()
	threadID := uuid.New()
	identityID := uuid.New()
	triggerID := uuid.New()
	now := time.Now().UTC()
	dmChannelID := "discord-dm-channel-1"

	if _, err := pool.Exec(ctx, `
		INSERT INTO accounts (id, slug, name, type) VALUES ($1, 'heartbeat-account', 'Heartbeat Account', 'personal');
		INSERT INTO projects (id, account_id, name, visibility, created_at) VALUES ($2, $1, 'Heartbeat Project', 'private', now());
		INSERT INTO personas (
			id, project_id, persona_key, version, display_name, prompt_md,
			tool_allowlist, tool_denylist, budgets_json, roles_json, title_summarize_json,
			is_active, prompt_cache_control, executor_type, executor_config_json, created_at, updated_at
		) VALUES (
			$3, $2, 'discord-persona', '1', 'Discord Persona', 'hello',
			'[]'::jsonb, '[]'::jsonb, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
			true, 'none', 'agent.simple', '{}'::jsonb, now(), now()
		);
		INSERT INTO channels (id, account_id, channel_type, persona_id, owner_user_id, webhook_secret, webhook_url, is_active, config_json)
		VALUES ($4, $1, 'discord', $3, NULL, 'whsec', 'https://example.com', true, '{}'::jsonb);
		INSERT INTO threads (id, account_id, created_by_user_id, project_id, title, is_private, created_at)
		VALUES ($5, $1, NULL, $2, 'Heartbeat Thread', false, now());
		INSERT INTO channel_identities (id, channel_type, platform_subject_id, display_name, metadata)
		VALUES ($6, 'discord', 'discord-user-1', 'Discord User', '{}'::jsonb);
		INSERT INTO channel_identity_links (
			id, channel_id, channel_identity_id,
			heartbeat_enabled, heartbeat_interval_minutes, heartbeat_model,
			created_at, updated_at
		) VALUES (
			gen_random_uuid(), $4, $6,
			1, 15, 'discord-model',
			now(), now()
		);
		INSERT INTO channel_dm_threads (channel_id, channel_identity_id, persona_id, thread_id)
		VALUES ($4, $6, $3, $5);
		INSERT INTO channel_message_ledger (
			channel_id, channel_type, direction, thread_id, run_id,
			platform_conversation_id, platform_message_id, platform_parent_message_id, platform_thread_id,
			sender_channel_identity_id, metadata_json, created_at
		) VALUES (
			$4, 'discord', 'inbound', $5, NULL,
			$7, 'msg-1', NULL, NULL,
			$6, '{}'::jsonb, now()
		);
		INSERT INTO scheduled_triggers (
			id, channel_id, channel_identity_id, persona_key, account_id, model, interval_min, next_fire_at, created_at, updated_at
		) VALUES (
			$8, $4, $6, 'discord-persona', $1, 'discord-model', 15, $9, $9, $9
		);`,
		accountID,
		projectID,
		personaID,
		channelID,
		threadID,
		identityID,
		dmChannelID,
		triggerID,
		now,
	); err != nil {
		t.Fatalf("seed heartbeat scheduler data: %v", err)
	}

	s := &TriggerScheduler{
		pool:     pool,
		jobs:     jobsRepo,
		runs:     runRepo,
		triggers: data.ScheduledTriggersRepository{},
	}
	s.fireOne(ctx, data.ScheduledTriggerRow{
		ID:                triggerID,
		ChannelID:         channelID,
		ChannelIdentityID: identityID,
		PersonaKey:        "discord-persona",
		AccountID:         accountID,
		Model:             "discord-model",
		IntervalMin:       15,
		NextFireAt:        now,
	})

	var payloadJSON []byte
	if err := pool.QueryRow(ctx, `
		SELECT payload_json
		  FROM jobs
		 WHERE job_type = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		data.RunExecuteJobType,
	).Scan(&payloadJSON); err != nil {
		t.Fatalf("read queued heartbeat job: %v", err)
	}

	var envelope struct {
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(payloadJSON, &envelope); err != nil {
		t.Fatalf("decode queued heartbeat payload: %v", err)
	}
	rawDelivery, ok := envelope.Payload["channel_delivery"].(map[string]any)
	if !ok {
		t.Fatalf("expected channel_delivery in payload: %#v", envelope.Payload)
	}
	if got := rawDelivery["channel_id"]; got != channelID.String() {
		t.Fatalf("channel_delivery.channel_id = %#v, want %s", got, channelID)
	}
	if got := rawDelivery["sender_channel_identity_id"]; got != identityID.String() {
		t.Fatalf("channel_delivery.sender_channel_identity_id = %#v, want %s", got, identityID)
	}
	if got := rawDelivery["conversation_type"]; got != "private" {
		t.Fatalf("channel_delivery.conversation_type = %#v, want private", got)
	}
	conversationRef, ok := rawDelivery["conversation_ref"].(map[string]any)
	if !ok {
		t.Fatalf("expected conversation_ref in channel_delivery: %#v", rawDelivery)
	}
	if got := conversationRef["target"]; got != dmChannelID {
		t.Fatalf("channel_delivery.conversation_ref.target = %#v, want %s", got, dmChannelID)
	}
}
