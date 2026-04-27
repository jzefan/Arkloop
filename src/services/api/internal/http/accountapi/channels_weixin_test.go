//go:build !desktop

package accountapi

import (
	"context"
	"encoding/json"
	"testing"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func TestCurrentWeixinChannelReloadsLatestConfig(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupPostgresDatabase(t, "api_go_weixin_channel_refresh")
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 4, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	accountRepo, err := data.NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("account repo: %v", err)
	}
	channelsRepo, err := data.NewChannelsRepository(pool)
	if err != nil {
		t.Fatalf("channels repo: %v", err)
	}
	account, err := accountRepo.Create(ctx, "weixin-refresh", "Weixin Refresh", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	oldConfig := json.RawMessage(`{"default_model":"old^model"}`)
	created, err := channelsRepo.Create(ctx, uuid.Nil, account.ID, "weixin", nil, nil, nil, "", "", oldConfig)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	active := true
	staleConfig := json.RawMessage(`{"default_model":"old^model"}`)
	stale, err := channelsRepo.Update(ctx, created.ID, account.ID, data.ChannelUpdate{
		IsActive:   &active,
		ConfigJSON: &staleConfig,
	})
	if err != nil {
		t.Fatalf("activate channel: %v", err)
	}
	newConfig := json.RawMessage(`{"default_model":"new^model"}`)
	if _, err := channelsRepo.Update(ctx, created.ID, account.ID, data.ChannelUpdate{ConfigJSON: &newConfig}); err != nil {
		t.Fatalf("update channel config: %v", err)
	}

	connector := &weixinConnector{channelsRepo: channelsRepo}
	got, ok, err := connector.currentWeixinChannel(ctx, *stale)
	if err != nil {
		t.Fatalf("current channel: %v", err)
	}
	if !ok {
		t.Fatal("expected active channel")
	}
	cfg, err := resolveWeixinChannelConfig(got.ConfigJSON)
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if cfg.DefaultModel != "new^model" {
		t.Fatalf("expected latest default model, got %q", cfg.DefaultModel)
	}
}
