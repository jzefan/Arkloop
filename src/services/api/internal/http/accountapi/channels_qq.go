package accountapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"strings"
	"time"

	"arkloop/services/api/internal/data"
	httpkit "arkloop/services/api/internal/http/httpkit"
	"arkloop/services/api/internal/observability"
	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/onebotclient"
	"arkloop/services/shared/pgnotify"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type qqChannelConfig struct {
	AllowedUserIDs  []string `json:"allowed_user_ids,omitempty"`
	AllowedGroupIDs []string `json:"allowed_group_ids,omitempty"`
	AllowAllUsers   bool     `json:"allow_all_users,omitempty"`
	DefaultModel    string   `json:"default_model,omitempty"`
	OneBotWSURL     string   `json:"onebot_ws_url,omitempty"`
	OneBotHTTPURL   string   `json:"onebot_http_url,omitempty"`
	OneBotToken     string   `json:"onebot_token,omitempty"`
}

func resolveQQChannelConfig(raw json.RawMessage) (qqChannelConfig, error) {
	if len(raw) == 0 {
		return qqChannelConfig{AllowAllUsers: true}, nil
	}
	var cfg qqChannelConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return qqChannelConfig{}, fmt.Errorf("invalid qq channel config: %w", err)
	}
	if len(cfg.AllowedUserIDs) == 0 && len(cfg.AllowedGroupIDs) == 0 {
		cfg.AllowAllUsers = true
	}
	return cfg, nil
}

func qqUserAllowed(cfg qqChannelConfig, userID, groupID string) bool {
	if cfg.AllowAllUsers {
		return true
	}
	if groupID != "" {
		for _, id := range cfg.AllowedGroupIDs {
			if id == groupID {
				return true
			}
		}
	}
	for _, id := range cfg.AllowedUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

type qqConnector struct {
	channelsRepo            *data.ChannelsRepository
	channelIdentitiesRepo   *data.ChannelIdentitiesRepository
	channelDMThreadsRepo    *data.ChannelDMThreadsRepository
	channelGroupThreadsRepo *data.ChannelGroupThreadsRepository
	channelReceiptsRepo     *data.ChannelMessageReceiptsRepository
	channelLedgerRepo       *data.ChannelMessageLedgerRepository
	personasRepo            *data.PersonasRepository
	threadRepo              *data.ThreadRepository
	messageRepo             *data.MessageRepository
	runEventRepo            *data.RunEventRepository
	jobRepo                 *data.JobRepository
	pool                    data.DB
	inputNotify             func(ctx context.Context, runID uuid.UUID)
}

// HandleEvent 处理来自 OneBot11 的入站事件
func (c *qqConnector) HandleEvent(ctx context.Context, traceID string, ch data.Channel, event onebotclient.Event) error {
	if !event.IsMessageEvent() {
		return nil
	}

	cfg, err := resolveQQChannelConfig(ch.ConfigJSON)
	if err != nil {
		return fmt.Errorf("invalid qq channel config: %w", err)
	}

	userID := event.UserID.String()
	groupID := event.GroupID.String()
	if groupID == "0" {
		groupID = ""
	}

	if !qqUserAllowed(cfg, userID, groupID) {
		return nil
	}

	text := strings.TrimSpace(event.PlainText())
	if text == "" {
		return nil
	}

	persona, personaRef, err := c.resolveQQPersona(ctx, ch)
	if err != nil {
		return err
	}

	isPrivate := event.IsPrivateMessage()
	platformChatID := userID
	chatType := "private"
	if !isPrivate {
		platformChatID = groupID
		chatType = "group"
	}
	platformMsgID := event.MessageID.String()

	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	accepted, err := c.channelReceiptsRepo.WithTx(tx).Record(ctx, ch.ID, platformChatID, platformMsgID)
	if err != nil {
		return err
	}
	if !accepted {
		return tx.Commit(ctx)
	}

	displayName := event.SenderDisplayName()
	if displayName == "" {
		displayName = userID
	}
	identity, err := c.channelIdentitiesRepo.WithTx(tx).Upsert(ctx, "qq", userID, &displayName, nil, nil)
	if err != nil {
		return err
	}

	if c.channelLedgerRepo != nil {
		ledgerMeta, _ := json.Marshal(map[string]any{
			"source":            "qq",
			"conversation_type": chatType,
		})
		if _, err := c.channelLedgerRepo.WithTx(tx).Record(ctx, data.ChannelMessageLedgerRecordInput{
			ChannelID:              ch.ID,
			ChannelType:            ch.ChannelType,
			Direction:              data.ChannelMessageDirectionInbound,
			PlatformConversationID: platformChatID,
			PlatformMessageID:      platformMsgID,
			SenderChannelIdentityID: &identity.ID,
			MetadataJSON:           ledgerMeta,
		}); err != nil {
			return err
		}
	}

	projection := buildQQEnvelopeText(identity.ID, displayName, chatType, text, event.Time)
	content, err := messagecontent.Normalize(messagecontent.FromText(projection).Parts)
	if err != nil {
		return err
	}
	contentJSON, err := content.JSON()
	if err != nil {
		return err
	}
	metadataJSON, _ := json.Marshal(map[string]any{
		"source":              "qq",
		"channel_identity_id": identity.ID.String(),
		"display_name":        displayName,
		"platform_chat_id":    platformChatID,
		"platform_message_id": platformMsgID,
		"platform_user_id":    userID,
		"chat_type":           chatType,
	})

	threadProjectID := derefUUID(persona.ProjectID)
	if threadProjectID == uuid.Nil {
		ownerUserID := uuid.Nil
		if ch.OwnerUserID != nil {
			ownerUserID = *ch.OwnerUserID
		}
		if ownerUserID == uuid.Nil {
			if identity.UserID != nil {
				ownerUserID = *identity.UserID
			}
		}
		if ownerUserID != uuid.Nil {
			if pid, err := c.personasRepo.GetOrCreateDefaultProjectIDByOwner(ctx, ch.AccountID, ownerUserID); err == nil {
				threadProjectID = pid
			}
		}
	}
	if threadProjectID == uuid.Nil {
		return fmt.Errorf("cannot resolve project for persona %s", persona.ID)
	}
	threadID, err := c.resolveQQThreadID(ctx, tx, ch, persona.ID, threadProjectID, identity, isPrivate, platformChatID)
	if err != nil {
		return err
	}

	if _, err := c.messageRepo.WithTx(tx).CreateStructuredWithMetadata(
		ctx, ch.AccountID, threadID, "user", projection, contentJSON, metadataJSON, identity.UserID,
	); err != nil {
		return err
	}

	runRepoTx := c.runEventRepo.WithTx(tx)
	if err := runRepoTx.LockThreadRow(ctx, threadID); err != nil {
		return err
	}
	if activeRun, err := runRepoTx.GetActiveRootRunForThread(ctx, threadID); err != nil {
		return err
	} else if activeRun != nil {
		delivered, err := c.deliverToActiveRun(ctx, runRepoTx, activeRun, projection, traceID)
		if err != nil {
			return err
		}
		if delivered {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			slog.InfoContext(ctx, "qq_inbound_processed",
				"stage", "delivered_to_existing_run",
				"channel_id", ch.ID, "run_id", activeRun.ID, "thread_id", threadID,
			)
			c.notifyInput(ctx, activeRun.ID)
			return nil
		}
	}

	if !channelAgentTriggerConsume(ch.ID) {
		return tx.Commit(ctx)
	}

	runData := map[string]any{"persona_id": personaRef}
	if m := strings.TrimSpace(cfg.DefaultModel); m != "" {
		runData["model"] = m
	}
	run, _, err := runRepoTx.CreateRunWithStartedEvent(ctx, ch.AccountID, threadID, identity.UserID, "run.started", runData)
	if err != nil {
		return err
	}

	jobPayload := map[string]any{
		"source": "qq",
		"channel_delivery": map[string]any{
			"channel_id":   ch.ID.String(),
			"channel_type": "qq",
			"conversation_ref": map[string]any{
				"target": platformChatID,
			},
			"inbound_message_ref": map[string]any{
				"message_id": platformMsgID,
			},
			"trigger_message_ref": map[string]any{
				"message_id": platformMsgID,
			},
			"platform_chat_id":           platformChatID,
			"platform_message_id":        platformMsgID,
			"sender_channel_identity_id": identity.ID.String(),
			"conversation_type":          chatType,
			"message_type":               chatType,
		},
	}
	if _, err := c.jobRepo.WithTx(tx).EnqueueRun(ctx, ch.AccountID, run.ID, traceID, data.RunExecuteJobType, jobPayload, nil); err != nil {
		return err
	}

	slog.InfoContext(ctx, "qq_inbound_processed",
		"stage", "new_run_enqueued",
		"channel_id", ch.ID, "run_id", run.ID, "thread_id", threadID,
	)

	return tx.Commit(ctx)
}

func (c *qqConnector) resolveQQPersona(ctx context.Context, ch data.Channel) (*data.Persona, string, error) {
	if ch.PersonaID == nil || *ch.PersonaID == uuid.Nil {
		return nil, "", fmt.Errorf("qq channel requires persona_id")
	}
	persona, err := c.personasRepo.GetByIDForAccount(ctx, ch.AccountID, *ch.PersonaID)
	if err != nil {
		return nil, "", err
	}
	if persona == nil || !persona.IsActive {
		return nil, "", fmt.Errorf("persona not found or inactive")
	}
	return persona, buildPersonaRef(*persona), nil
}

func (c *qqConnector) resolveQQThreadID(
	ctx context.Context, tx pgx.Tx, ch data.Channel,
	personaID, projectID uuid.UUID, identity data.ChannelIdentity,
	isPrivate bool, platformChatID string,
) (uuid.UUID, error) {
	if isPrivate {
		threadMap, err := c.channelDMThreadsRepo.WithTx(tx).GetByBinding(ctx, ch.ID, identity.ID, personaID)
		if err != nil {
			return uuid.Nil, err
		}
		if threadMap != nil {
			return threadMap.ThreadID, nil
		}
		thread, err := c.threadRepo.WithTx(tx).Create(ctx, ch.AccountID, identity.UserID, projectID, nil, false)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := c.channelDMThreadsRepo.WithTx(tx).Create(ctx, ch.ID, identity.ID, personaID, thread.ID); err != nil {
			return uuid.Nil, err
		}
		return thread.ID, nil
	}

	threadMap, err := c.channelGroupThreadsRepo.WithTx(tx).GetByBinding(ctx, ch.ID, platformChatID, personaID)
	if err != nil {
		return uuid.Nil, err
	}
	if threadMap != nil {
		return threadMap.ThreadID, nil
	}
	thread, err := c.threadRepo.WithTx(tx).Create(ctx, ch.AccountID, nil, projectID, nil, false)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := c.channelGroupThreadsRepo.WithTx(tx).Create(ctx, ch.ID, platformChatID, personaID, thread.ID); err != nil {
		return uuid.Nil, err
	}
	return thread.ID, nil
}

func (c *qqConnector) deliverToActiveRun(ctx context.Context, repo *data.RunEventRepository, run *data.Run, content, traceID string) (bool, error) {
	if run == nil || strings.TrimSpace(content) == "" {
		return false, nil
	}
	if _, err := repo.ProvideInput(ctx, run.ID, content, traceID); err != nil {
		var notActive data.RunNotActiveError
		if errors.As(err, &notActive) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *qqConnector) notifyInput(ctx context.Context, runID uuid.UUID) {
	if c.inputNotify == nil || runID == uuid.Nil {
		return
	}
	c.inputNotify(ctx, runID)
}

func buildQQEnvelopeText(identityID uuid.UUID, displayName, chatType, body string, unixTS int64) string {
	ts := ""
	if unixTS > 0 {
		ts = time.Unix(unixTS, 0).UTC().Format(time.RFC3339)
	}
	lines := []string{
		fmt.Sprintf(`display-name: "%s"`, escapeEnvelopeValue(displayName)),
		`channel: "qq"`,
		fmt.Sprintf(`conversation-type: "%s"`, chatType),
	}
	if identityID != uuid.Nil {
		lines = append(lines, fmt.Sprintf(`sender-ref: "%s"`, identityID.String()))
	}
	if ts != "" {
		lines = append(lines, fmt.Sprintf(`time: "%s"`, ts))
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + body
}

func escapeEnvelopeValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.TrimSpace(value))
}

// qqOneBotCallbackHandler 返回处理 NapCat HTTP Client 回调的 handler
func qqOneBotCallbackHandler(
	channelsRepo *data.ChannelsRepository,
	channelIdentitiesRepo *data.ChannelIdentitiesRepository,
	channelDMThreadsRepo *data.ChannelDMThreadsRepository,
	channelGroupThreadsRepo *data.ChannelGroupThreadsRepository,
	channelReceiptsRepo *data.ChannelMessageReceiptsRepository,
	personasRepo *data.PersonasRepository,
	threadRepo *data.ThreadRepository,
	messageRepo *data.MessageRepository,
	runEventRepo *data.RunEventRepository,
	jobRepo *data.JobRepository,
	pool data.DB,
) nethttp.HandlerFunc {
	var channelLedgerRepo *data.ChannelMessageLedgerRepository
	if pool != nil {
		repo, err := data.NewChannelMessageLedgerRepository(pool)
		if err != nil {
			panic(err)
		}
		channelLedgerRepo = repo
	}

	connector := &qqConnector{
		channelsRepo:            channelsRepo,
		channelIdentitiesRepo:   channelIdentitiesRepo,
		channelDMThreadsRepo:    channelDMThreadsRepo,
		channelGroupThreadsRepo: channelGroupThreadsRepo,
		channelReceiptsRepo:     channelReceiptsRepo,
		channelLedgerRepo:       channelLedgerRepo,
		personasRepo:            personasRepo,
		threadRepo:              threadRepo,
		messageRepo:             messageRepo,
		runEventRepo:            runEventRepo,
		jobRepo:                 jobRepo,
		pool:                    pool,
		inputNotify: func(ctx context.Context, runID uuid.UUID) {
			if _, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgnotify.ChannelRunInput, runID.String()); err != nil {
				slog.Warn("qq_active_run_notify_failed", "run_id", runID, "error", err)
			}
		},
	}

	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		var event onebotclient.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			httpkit.WriteError(w, nethttp.StatusBadRequest, "validation.error", "invalid onebot event", traceID, nil)
			return
		}

		if event.IsHeartbeat() || event.IsLifecycle() {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
			return
		}

		if !event.IsMessageEvent() {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
			return
		}

		channels, err := channelsRepo.ListActiveByType(r.Context(), "qq")
		if err != nil {
			httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}
		if len(channels) == 0 {
			httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
			return
		}

		// 对第一个活跃的 QQ channel 处理事件
		ch := channels[0]
		if err := connector.HandleEvent(r.Context(), traceID, ch, event); err != nil {
			slog.ErrorContext(r.Context(), "qq_onebot_callback_error", "error", err, "channel_id", ch.ID)
			httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}

		httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
	}
}
