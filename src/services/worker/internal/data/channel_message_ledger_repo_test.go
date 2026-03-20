//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLookupByPlatformMessage(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupPostgresDatabase(t, "ledger_lookup")
	pool, err := pgxpool.New(ctx, db.DSN)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	channelID := uuid.New()
	threadID := uuid.New()
	runID := uuid.New()

	if _, err := pool.Exec(ctx, `
CREATE TABLE channel_message_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL,
    channel_type TEXT NOT NULL,
    direction TEXT NOT NULL,
    thread_id UUID NULL,
    run_id UUID NULL,
    platform_conversation_id TEXT NOT NULL,
    platform_message_id TEXT NOT NULL,
    platform_parent_message_id TEXT NULL,
    platform_thread_id TEXT NULL,
    sender_channel_identity_id UUID NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_direction CHECK (direction IN ('inbound', 'outbound')),
    CONSTRAINT uq_entry UNIQUE (channel_id, direction, platform_conversation_id, platform_message_id)
)`); err != nil {
		t.Fatal(err)
	}

	repo := ChannelMessageLedgerRepository{}
	if err := repo.Record(ctx, pool, ChannelMessageLedgerRecordInput{
		ChannelID:              channelID,
		ChannelType:            "telegram",
		Direction:              ChannelMessageDirectionInbound,
		ThreadID:               &threadID,
		RunID:                  &runID,
		PlatformConversationID: "chat-1",
		PlatformMessageID:      "msg-99",
	}); err != nil {
		t.Fatal(err)
	}

	row, err := repo.LookupByPlatformMessage(ctx, pool, channelID, "chat-1", "msg-99")
	if err != nil {
		t.Fatal(err)
	}
	if row == nil {
		t.Fatal("expected row")
	}
	if row.Direction != ChannelMessageDirectionInbound || row.PlatformMessageID != "msg-99" {
		t.Fatalf("row: %+v", row)
	}

	miss, err := repo.LookupByPlatformMessage(ctx, pool, channelID, "chat-1", "nope")
	if err != nil {
		t.Fatal(err)
	}
	if miss != nil {
		t.Fatalf("expected nil, got %+v", miss)
	}
}
