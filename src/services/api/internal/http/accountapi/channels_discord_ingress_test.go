//go:build !desktop

package accountapi

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"arkloop/services/api/internal/auth"
	apiCrypto "arkloop/services/api/internal/crypto"
	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
	"arkloop/services/shared/discordbot"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type discordIngressTestEnv struct {
	handler               nethttp.Handler
	pool                  *pgxpool.Pool
	accessToken           string
	accountID             uuid.UUID
	userID                uuid.UUID
	personaID             uuid.UUID
	projectID             uuid.UUID
	channelsRepo          *data.ChannelsRepository
	channelIdentitiesRepo *data.ChannelIdentitiesRepository
	channelBindCodesRepo  *data.ChannelBindCodesRepository
	channelDMThreadsRepo  *data.ChannelDMThreadsRepository
	channelReceiptsRepo   *data.ChannelMessageReceiptsRepository
	channelLedgerRepo     *data.ChannelMessageLedgerRepository
	personasRepo          *data.PersonasRepository
	secretsRepo           *data.SecretsRepository
	threadRepo            *data.ThreadRepository
	messageRepo           *data.MessageRepository
	runEventRepo          *data.RunEventRepository
	jobRepo               *data.JobRepository
	creditsRepo           *data.CreditsRepository
}

func setupDiscordIngressTestEnv(t *testing.T) discordIngressTestEnv {
	return setupDiscordIngressTestEnvWithClient(t, nil)
}

func setupDiscordIngressTestEnvWithClient(t *testing.T, botClient *discordbot.Client) discordIngressTestEnv {
	t.Helper()

	db := testutil.SetupPostgresDatabase(t, "api_go_channels_discord_ingress")
	if _, err := migrate.Up(context.Background(), db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := data.NewPool(context.Background(), db.DSN, data.PoolLimits{MaxConns: 16, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	channelRunTriggerLog.Lock()
	channelRunTriggerByChannel = map[uuid.UUID][]time.Time{}
	channelRunTriggerLog.Unlock()

	accountRepo, err := data.NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("account repo: %v", err)
	}
	userRepo, err := data.NewUserRepository(pool)
	if err != nil {
		t.Fatalf("user repo: %v", err)
	}
	userCredRepo, err := data.NewUserCredentialRepository(pool)
	if err != nil {
		t.Fatalf("user credential repo: %v", err)
	}
	refreshTokenRepo, err := data.NewRefreshTokenRepository(pool)
	if err != nil {
		t.Fatalf("refresh token repo: %v", err)
	}
	membershipRepo, err := data.NewAccountMembershipRepository(pool)
	if err != nil {
		t.Fatalf("membership repo: %v", err)
	}
	projectRepo, err := data.NewProjectRepository(pool)
	if err != nil {
		t.Fatalf("project repo: %v", err)
	}
	personasRepo, err := data.NewPersonasRepository(pool)
	if err != nil {
		t.Fatalf("personas repo: %v", err)
	}
	channelsRepo, err := data.NewChannelsRepository(pool)
	if err != nil {
		t.Fatalf("channels repo: %v", err)
	}
	channelIdentitiesRepo, err := data.NewChannelIdentitiesRepository(pool)
	if err != nil {
		t.Fatalf("channel identities repo: %v", err)
	}
	channelBindCodesRepo, err := data.NewChannelBindCodesRepository(pool)
	if err != nil {
		t.Fatalf("bind repo: %v", err)
	}
	channelDMThreadsRepo, err := data.NewChannelDMThreadsRepository(pool)
	if err != nil {
		t.Fatalf("dm threads repo: %v", err)
	}
	channelReceiptsRepo, err := data.NewChannelMessageReceiptsRepository(pool)
	if err != nil {
		t.Fatalf("receipts repo: %v", err)
	}
	channelLedgerRepo, err := data.NewChannelMessageLedgerRepository(pool)
	if err != nil {
		t.Fatalf("ledger repo: %v", err)
	}
	threadRepo, err := data.NewThreadRepository(pool)
	if err != nil {
		t.Fatalf("thread repo: %v", err)
	}
	messageRepo, err := data.NewMessageRepository(pool)
	if err != nil {
		t.Fatalf("message repo: %v", err)
	}
	runEventRepo, err := data.NewRunEventRepository(pool)
	if err != nil {
		t.Fatalf("run repo: %v", err)
	}
	jobRepo, err := data.NewJobRepository(pool)
	if err != nil {
		t.Fatalf("job repo: %v", err)
	}
	creditsRepo, err := data.NewCreditsRepository(pool)
	if err != nil {
		t.Fatalf("credits repo: %v", err)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	keyRing, err := apiCrypto.NewKeyRing(map[int][]byte{1: key})
	if err != nil {
		t.Fatalf("key ring: %v", err)
	}
	secretsRepo, err := data.NewSecretsRepository(pool, keyRing)
	if err != nil {
		t.Fatalf("secrets repo: %v", err)
	}

	account, err := accountRepo.Create(context.Background(), "discord-account", "Discord Account", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	user, err := userRepo.Create(context.Background(), "discord-owner", "discord-owner@test.com", "zh")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := membershipRepo.Create(context.Background(), account.ID, user.ID, auth.RoleAccountAdmin); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	project, err := projectRepo.CreateDefaultForOwner(context.Background(), account.ID, user.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	persona, err := personasRepo.Create(
		context.Background(),
		project.ID,
		"discord-persona",
		"1",
		"Discord Persona",
		nil,
		"hello",
		nil,
		nil,
		json.RawMessage(`{}`),
		json.RawMessage(`{}`),
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

	passwordHasher, err := auth.NewBcryptPasswordHasher(0)
	if err != nil {
		t.Fatalf("password hasher: %v", err)
	}
	tokenService, err := auth.NewJwtAccessTokenService("discord-test-secret-should-be-long-enough", 3600, 2592000)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	authService, err := auth.NewService(userRepo, userCredRepo, membershipRepo, passwordHasher, tokenService, refreshTokenRepo, nil, projectRepo)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	accessToken, err := tokenService.Issue(user.ID, account.ID, auth.RoleAccountAdmin, time.Now().UTC())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	mux := nethttp.NewServeMux()
	RegisterRoutes(mux, Deps{
		AuthService:           authService,
		AccountMembershipRepo: membershipRepo,
		ThreadRepo:            threadRepo,
		ProjectRepo:           projectRepo,
		Pool:                  pool,
		AccountRepo:           accountRepo,
		SecretsRepo:           secretsRepo,
		ChannelsRepo:          channelsRepo,
		ChannelIdentitiesRepo: channelIdentitiesRepo,
		ChannelBindCodesRepo:  channelBindCodesRepo,
		ChannelDMThreadsRepo:  channelDMThreadsRepo,
		ChannelReceiptsRepo:   channelReceiptsRepo,
		UsersRepo:             userRepo,
		MessageRepo:           messageRepo,
		RunEventRepo:          runEventRepo,
		JobRepo:               jobRepo,
		CreditsRepo:           creditsRepo,
		PersonasRepo:          personasRepo,
		AppBaseURL:            "https://app.example",
		DiscordBotClient:      botClient,
	})

	return discordIngressTestEnv{
		handler:               mux,
		pool:                  pool,
		accessToken:           accessToken,
		accountID:             account.ID,
		userID:                user.ID,
		personaID:             persona.ID,
		projectID:             project.ID,
		channelsRepo:          channelsRepo,
		channelIdentitiesRepo: channelIdentitiesRepo,
		channelBindCodesRepo:  channelBindCodesRepo,
		channelDMThreadsRepo:  channelDMThreadsRepo,
		channelReceiptsRepo:   channelReceiptsRepo,
		channelLedgerRepo:     channelLedgerRepo,
		personasRepo:          personasRepo,
		secretsRepo:           secretsRepo,
		threadRepo:            threadRepo,
		messageRepo:           messageRepo,
		runEventRepo:          runEventRepo,
		jobRepo:               jobRepo,
		creditsRepo:           creditsRepo,
	}
}

func (e discordIngressTestEnv) createActiveDiscordChannel(t *testing.T, config json.RawMessage) data.Channel {
	t.Helper()
	return e.createActiveDiscordChannelWithToken(t, config, "")
}

func (e discordIngressTestEnv) createActiveDiscordChannelWithToken(t *testing.T, config json.RawMessage, botToken string) data.Channel {
	t.Helper()

	var credentialsID *uuid.UUID
	channelID := uuid.New()
	if botToken != "" {
		secret, err := e.secretsRepo.Create(context.Background(), e.userID, data.ChannelSecretName(channelID), botToken)
		if err != nil {
			t.Fatalf("create secret: %v", err)
		}
		credentialsID = &secret.ID
	}

	ch, err := e.channelsRepo.Create(context.Background(), channelID, e.accountID, "discord", &e.personaID, credentialsID, &e.userID, "", "", config)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}
	active := true
	updated, err := e.channelsRepo.Update(context.Background(), ch.ID, e.accountID, data.ChannelUpdate{IsActive: &active})
	if err != nil {
		t.Fatalf("activate channel: %v", err)
	}
	if updated == nil {
		t.Fatal("activate channel returned nil")
	}
	return *updated
}

func (e discordIngressTestEnv) connector() discordConnector {
	return discordConnector{
		channelsRepo:          e.channelsRepo,
		channelIdentitiesRepo: e.channelIdentitiesRepo,
		channelBindCodesRepo:  e.channelBindCodesRepo,
		channelDMThreadsRepo:  e.channelDMThreadsRepo,
		channelReceiptsRepo:   e.channelReceiptsRepo,
		channelLedgerRepo:     e.channelLedgerRepo,
		personasRepo:          e.personasRepo,
		threadRepo:            e.threadRepo,
		messageRepo:           e.messageRepo,
		runEventRepo:          e.runEventRepo,
		jobRepo:               e.jobRepo,
		creditsRepo:           e.creditsRepo,
		pool:                  e.pool,
	}
}

func newDiscordMessageCreate(messageID, channelID, userID, username, content string, replyTo *string) *discordgo.MessageCreate {
	var reference *discordgo.MessageReference
	if replyTo != nil && *replyTo != "" {
		reference = &discordgo.MessageReference{MessageID: *replyTo}
	}
	return &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:               messageID,
			ChannelID:        channelID,
			Content:          content,
			Author:           &discordgo.User{ID: userID, Username: username},
			MessageReference: reference,
			Timestamp:        time.Now().UTC(),
		},
	}
}

func newDiscordInteractionCommand(name, guildID, channelID, userID, username, code string) *discordgo.InteractionCreate {
	dataJSON := discordgo.ApplicationCommandInteractionData{
		Name:        name,
		CommandType: discordgo.ChatApplicationCommand,
	}
	if code != "" {
		dataJSON.Options = []*discordgo.ApplicationCommandInteractionDataOption{{
			Name:  "code",
			Type:  discordgo.ApplicationCommandOptionString,
			Value: code,
		}}
	}
	return &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type:      discordgo.InteractionApplicationCommand,
			GuildID:   guildID,
			ChannelID: channelID,
			User:      &discordgo.User{ID: userID, Username: username},
			Data:      dataJSON,
		},
	}
}

func TestDiscordHandleMessageCreateCreatesRunAndInboundLedger(t *testing.T) {
	env := setupDiscordIngressTestEnv(t)
	channel := env.createActiveDiscordChannel(t, json.RawMessage(`{"default_model":"openai^gpt-4.1-mini"}`))

	err := env.connector().HandleMessageCreate(
		context.Background(),
		"trace-discord-first",
		channel.ID,
		"",
		newDiscordMessageCreate("m-1", "dm-1", "u-1", "alice", "hello", nil),
	)
	if err != nil {
		t.Fatalf("handle message create: %v", err)
	}

	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM channel_identities`, 1)
	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM channel_dm_threads`, 1)
	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM channel_message_ledger WHERE direction = 'inbound'`, 1)
	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM messages`, 1)
	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM runs`, 1)
	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM jobs WHERE job_type = 'run.execute'`, 1)

	var startedJSON []byte
	if err := env.pool.QueryRow(context.Background(), `SELECT data_json::text::jsonb FROM run_events WHERE type = 'run.started' LIMIT 1`).Scan(&startedJSON); err != nil {
		t.Fatalf("query run.started: %v", err)
	}
	var started map[string]any
	if err := json.Unmarshal(startedJSON, &started); err != nil {
		t.Fatalf("decode run.started: %v", err)
	}
	if got := asString(started["model"]); got != "openai^gpt-4.1-mini" {
		t.Fatalf("unexpected model: %q", got)
	}
}

func TestDiscordHandleMessageCreateAppendsToActiveRun(t *testing.T) {
	env := setupDiscordIngressTestEnv(t)
	channel := env.createActiveDiscordChannel(t, json.RawMessage(`{}`))
	identity, err := upsertDiscordIdentity(context.Background(), env.channelIdentitiesRepo, &discordgo.User{ID: "u-append", Username: "append-user"})
	if err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	thread, err := env.threadRepo.Create(context.Background(), env.accountID, identity.UserID, env.projectID, nil, false)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := env.channelDMThreadsRepo.Create(context.Background(), channel.ID, identity.ID, env.personaID, thread.ID); err != nil {
		t.Fatalf("create thread binding: %v", err)
	}
	run, _, err := env.runEventRepo.CreateRunWithStartedEvent(context.Background(), env.accountID, thread.ID, identity.UserID, "run.started", map[string]any{"persona_id": "discord-persona@1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	err = env.connector().HandleMessageCreate(
		context.Background(),
		"trace-discord-active",
		channel.ID,
		"",
		newDiscordMessageCreate("m-2", "dm-2", "u-append", "append-user", "follow-up", nil),
	)
	if err != nil {
		t.Fatalf("handle message create: %v", err)
	}

	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM runs`, 1)
	assertCountAccount(t, env.pool, `SELECT COUNT(*) FROM jobs WHERE job_type = 'run.execute'`, 0)

	var content string
	if err := env.pool.QueryRow(context.Background(), `SELECT data_json->>'content' FROM run_events WHERE run_id = $1 AND type = 'run.input_provided' LIMIT 1`, run.ID).Scan(&content); err != nil {
		t.Fatalf("query run.input_provided: %v", err)
	}
	if content == "" || !json.Valid([]byte(`"`+content+`"`)) {
		t.Fatalf("expected non-empty provided content, got %q", content)
	}
}

func TestDiscordHandleInteractionBindConsumesCode(t *testing.T) {
	env := setupDiscordIngressTestEnv(t)
	channel := env.createActiveDiscordChannel(t, json.RawMessage(`{}`))
	channelType := "discord"
	code, err := env.channelBindCodesRepo.Create(context.Background(), env.userID, &channelType, time.Hour)
	if err != nil {
		t.Fatalf("create bind code: %v", err)
	}

	reply, err := env.connector().HandleInteraction(
		context.Background(),
		"trace-discord-bind",
		channel.ID,
		"",
		newDiscordInteractionCommand("bind", "", "dm-bind", "u-bind", "bind-user", code.Token),
	)
	if err != nil {
		t.Fatalf("handle bind interaction: %v", err)
	}
	if reply == nil || reply.Content != "绑定成功。" {
		t.Fatalf("unexpected reply: %#v", reply)
	}

	identity, err := env.channelIdentitiesRepo.GetByChannelAndSubject(context.Background(), "discord", "u-bind")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if identity == nil || identity.UserID == nil || *identity.UserID != env.userID {
		t.Fatalf("expected bound identity to belong to %s, got %#v", env.userID, identity)
	}

	activeCode, err := env.channelBindCodesRepo.GetActiveByToken(context.Background(), code.Token)
	if err != nil {
		t.Fatalf("get active bind code: %v", err)
	}
	if activeCode != nil {
		t.Fatalf("expected bind code consumed, got %#v", activeCode)
	}
}

func TestDiscordHandleInteractionNewRemovesDMThreadBinding(t *testing.T) {
	env := setupDiscordIngressTestEnv(t)
	channel := env.createActiveDiscordChannel(t, json.RawMessage(`{}`))
	identity, err := upsertDiscordIdentity(context.Background(), env.channelIdentitiesRepo, &discordgo.User{ID: "u-new", Username: "new-user"})
	if err != nil {
		t.Fatalf("upsert identity: %v", err)
	}
	thread, err := env.threadRepo.Create(context.Background(), env.accountID, identity.UserID, env.projectID, nil, false)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := env.channelDMThreadsRepo.Create(context.Background(), channel.ID, identity.ID, env.personaID, thread.ID); err != nil {
		t.Fatalf("create binding: %v", err)
	}

	reply, err := env.connector().HandleInteraction(
		context.Background(),
		"trace-discord-new",
		channel.ID,
		"",
		newDiscordInteractionCommand("new", "", "dm-new", "u-new", "new-user", ""),
	)
	if err != nil {
		t.Fatalf("handle new interaction: %v", err)
	}
	if reply == nil || reply.Content != "已开启新会话。" {
		t.Fatalf("unexpected reply: %#v", reply)
	}

	binding, err := env.channelDMThreadsRepo.GetByBinding(context.Background(), channel.ID, identity.ID, env.personaID)
	if err != nil {
		t.Fatalf("get binding: %v", err)
	}
	if binding != nil {
		t.Fatalf("expected binding deleted, got %#v", binding)
	}
}

func TestDiscordHandleInteractionRejectsGuildOutsideAllowlist(t *testing.T) {
	env := setupDiscordIngressTestEnv(t)
	channel := env.createActiveDiscordChannel(t, json.RawMessage(`{"allowed_server_ids":["guild-allow"],"allowed_channel_ids":["channel-allow"]}`))

	reply, err := env.connector().HandleInteraction(
		context.Background(),
		"trace-discord-allow",
		channel.ID,
		"",
		newDiscordInteractionCommand("help", "guild-deny", "channel-deny", "u-guild", "guild-user", ""),
	)
	if err != nil {
		t.Fatalf("handle guild interaction: %v", err)
	}
	if reply == nil || reply.Content != "当前服务器或频道未被授权。" || !reply.Ephemeral {
		t.Fatalf("unexpected reply: %#v", reply)
	}

	identity, err := env.channelIdentitiesRepo.GetByChannelAndSubject(context.Background(), "discord", "u-guild")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if identity != nil {
		t.Fatalf("expected no identity for denied guild command, got %#v", identity)
	}
}

func TestDiscordVerifyChannelBackfillsApplicationMetadata(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		switch r.URL.Path {
		case "/users/@me":
			_, _ = io.WriteString(w, `{"id":"bot-user-1","username":"arkloop-bot","bot":true}`)
		case "/applications/@me":
			_, _ = io.WriteString(w, `{"id":"app-1","name":"Arkloop Discord"}`)
		default:
			w.WriteHeader(nethttp.StatusNotFound)
		}
	}))
	defer server.Close()

	env := setupDiscordIngressTestEnvWithClient(t, discordbot.NewClient(server.URL, server.Client()))
	channel := env.createActiveDiscordChannelWithToken(t, json.RawMessage(`{}`), "discord-token")

	resp := doJSONAccount(
		env.handler,
		nethttp.MethodPost,
		"/v1/channels/"+channel.ID.String()+"/verify",
		nil,
		authHeader(env.accessToken),
	)
	if resp.Code != nethttp.StatusOK {
		t.Fatalf("verify status: %d %s", resp.Code, resp.Body.String())
	}

	got := decodeJSONBodyAccount[channelVerifyResponse](t, resp.Body.Bytes())
	if !got.OK {
		t.Fatalf("expected verify ok, got %#v", got)
	}
	if got.BotUsername != "arkloop-bot" || got.BotUserID != "bot-user-1" {
		t.Fatalf("unexpected bot metadata: %#v", got)
	}
	if got.ApplicationID != "app-1" || got.ApplicationName != "Arkloop Discord" {
		t.Fatalf("unexpected app metadata: %#v", got)
	}

	updated, err := env.channelsRepo.GetByID(context.Background(), channel.ID)
	if err != nil {
		t.Fatalf("get updated channel: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated channel")
	}
	var cfg discordChannelConfig
	if err := json.Unmarshal(updated.ConfigJSON, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.DiscordApplicationID != "app-1" || cfg.DiscordBotUserID != "bot-user-1" {
		t.Fatalf("expected config backfill, got %#v", cfg)
	}
}
