//go:build !desktop

package data

import (
	"context"
	"encoding/json"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

type channelDMThreadsRepoTestEnv struct {
	repo       *ChannelDMThreadsRepository
	ctx        context.Context
	accountID  uuid.UUID
	userID     uuid.UUID
	projectID  uuid.UUID
	personaID  uuid.UUID
	channelID  uuid.UUID
	identityID uuid.UUID
	threadRepo *ThreadRepository
}

func setupChannelDMThreadsRepoTestEnv(t *testing.T) channelDMThreadsRepoTestEnv {
	t.Helper()

	db := testutil.SetupPostgresDatabase(t, "api_go_channel_dm_threads")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo, err := NewChannelDMThreadsRepository(pool)
	if err != nil {
		t.Fatalf("new channel dm threads repo: %v", err)
	}
	accountRepo, err := NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("new account repo: %v", err)
	}
	userRepo, err := NewUserRepository(pool)
	if err != nil {
		t.Fatalf("new user repo: %v", err)
	}
	projectRepo, err := NewProjectRepository(pool)
	if err != nil {
		t.Fatalf("new project repo: %v", err)
	}
	personasRepo, err := NewPersonasRepository(pool)
	if err != nil {
		t.Fatalf("new personas repo: %v", err)
	}
	channelsRepo, err := NewChannelsRepository(pool)
	if err != nil {
		t.Fatalf("new channels repo: %v", err)
	}
	identitiesRepo, err := NewChannelIdentitiesRepository(pool)
	if err != nil {
		t.Fatalf("new identities repo: %v", err)
	}
	threadRepo, err := NewThreadRepository(pool)
	if err != nil {
		t.Fatalf("new thread repo: %v", err)
	}

	account, err := accountRepo.Create(ctx, "channel-dm-threads", "Channel DM Threads", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	user, err := userRepo.Create(ctx, "channel-dm-owner", "dm-owner@test.com", "zh")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	project, err := projectRepo.CreateDefaultForOwner(ctx, account.ID, user.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	persona, err := personasRepo.Create(
		ctx,
		project.ID,
		"channel-dm-persona",
		"1",
		"Channel DM Persona",
		nil,
		"hello",
		nil,
		nil,
		json.RawMessage(`{}`),
		json.RawMessage(`{}`),
		nil,
		nil,
		nil,
		"auto",
		true,
		"none",
		"agent.simple",
		nil,
	)
	if err != nil {
		t.Fatalf("create persona: %v", err)
	}
	channel, err := channelsRepo.Create(ctx, uuid.New(), account.ID, "telegram", &persona.ID, nil, &user.ID, "secret", "https://example.com/webhook", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	identity, err := identitiesRepo.Upsert(ctx, "telegram", "telegram-user-1", nil, nil, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}

	return channelDMThreadsRepoTestEnv{
		repo:       repo,
		ctx:        ctx,
		accountID:  account.ID,
		userID:     user.ID,
		projectID:  project.ID,
		personaID:  persona.ID,
		channelID:  channel.ID,
		identityID: identity.ID,
		threadRepo: threadRepo,
	}
}

func createChannelDMThreadTestThread(t *testing.T, env channelDMThreadsRepoTestEnv) uuid.UUID {
	t.Helper()
	userID := env.userID
	thread, err := env.threadRepo.Create(env.ctx, env.accountID, &userID, env.projectID, nil, false)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	return thread.ID
}

func TestChannelDMThreadsRepositoryTracksBindingsPerPlatformThread(t *testing.T) {
	env := setupChannelDMThreadsRepoTestEnv(t)

	mainThreadID := createChannelDMThreadTestThread(t, env)
	topicThreadID := createChannelDMThreadTestThread(t, env)

	mainBinding, err := env.repo.Create(env.ctx, env.channelID, env.identityID, env.personaID, "", mainThreadID)
	if err != nil {
		t.Fatalf("create main binding: %v", err)
	}
	if mainBinding.PlatformThreadID != "" {
		t.Fatalf("expected empty platform thread for main dm, got %q", mainBinding.PlatformThreadID)
	}

	topicBinding, err := env.repo.Create(env.ctx, env.channelID, env.identityID, env.personaID, "42", topicThreadID)
	if err != nil {
		t.Fatalf("create topic binding: %v", err)
	}
	if topicBinding.PlatformThreadID != "42" {
		t.Fatalf("unexpected platform thread id: %q", topicBinding.PlatformThreadID)
	}

	gotMain, err := env.repo.GetByBinding(env.ctx, env.channelID, env.identityID, env.personaID, "")
	if err != nil {
		t.Fatalf("get main binding: %v", err)
	}
	if gotMain == nil || gotMain.ThreadID != mainThreadID {
		t.Fatalf("unexpected main binding: %#v", gotMain)
	}

	gotTopic, err := env.repo.GetByBinding(env.ctx, env.channelID, env.identityID, env.personaID, "42")
	if err != nil {
		t.Fatalf("get topic binding: %v", err)
	}
	if gotTopic == nil || gotTopic.ThreadID != topicThreadID {
		t.Fatalf("unexpected topic binding: %#v", gotTopic)
	}

	allBindings, err := env.repo.ListByChannelIdentity(env.ctx, env.channelID, env.identityID)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(allBindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(allBindings))
	}
}

func TestChannelDMThreadsRepositoryDeleteByBindingOnlyRemovesRequestedTopic(t *testing.T) {
	env := setupChannelDMThreadsRepoTestEnv(t)

	mainThreadID := createChannelDMThreadTestThread(t, env)
	topicThreadID := createChannelDMThreadTestThread(t, env)
	if _, err := env.repo.Create(env.ctx, env.channelID, env.identityID, env.personaID, "", mainThreadID); err != nil {
		t.Fatalf("create main binding: %v", err)
	}
	if _, err := env.repo.Create(env.ctx, env.channelID, env.identityID, env.personaID, "42", topicThreadID); err != nil {
		t.Fatalf("create topic binding: %v", err)
	}

	if err := env.repo.DeleteByBinding(env.ctx, env.channelID, env.identityID, env.personaID, "42"); err != nil {
		t.Fatalf("delete topic binding: %v", err)
	}

	mainBinding, err := env.repo.GetByBinding(env.ctx, env.channelID, env.identityID, env.personaID, "")
	if err != nil {
		t.Fatalf("get main binding after delete: %v", err)
	}
	if mainBinding == nil || mainBinding.ThreadID != mainThreadID {
		t.Fatalf("main binding should remain, got %#v", mainBinding)
	}

	topicBinding, err := env.repo.GetByBinding(env.ctx, env.channelID, env.identityID, env.personaID, "42")
	if err != nil {
		t.Fatalf("get topic binding after delete: %v", err)
	}
	if topicBinding != nil {
		t.Fatalf("expected topic binding removed, got %#v", topicBinding)
	}
}
