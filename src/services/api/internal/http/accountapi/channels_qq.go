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
	"arkloop/services/api/internal/entitlement"
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
	BotQQ           string   `json:"bot_qq,omitempty"`
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
	channelsRepo             *data.ChannelsRepository
	channelIdentitiesRepo    *data.ChannelIdentitiesRepository
	channelBindCodesRepo     *data.ChannelBindCodesRepository
	channelIdentityLinksRepo *data.ChannelIdentityLinksRepository
	channelDMThreadsRepo     *data.ChannelDMThreadsRepository
	channelGroupThreadsRepo  *data.ChannelGroupThreadsRepository
	channelReceiptsRepo      *data.ChannelMessageReceiptsRepository
	channelLedgerRepo        *data.ChannelMessageLedgerRepository
	personasRepo             *data.PersonasRepository
	threadRepo               *data.ThreadRepository
	messageRepo              *data.MessageRepository
	runEventRepo             *data.RunEventRepository
	jobRepo                  *data.JobRepository
	entitlementSvc           *entitlement.Service
	pool                     data.DB
	inputNotify              func(ctx context.Context, runID uuid.UUID)
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

	isPrivate := event.IsPrivateMessage()
	platformChatID := userID
	chatType := "private"
	if !isPrivate {
		platformChatID = groupID
		chatType = "group"
	}
	platformMsgID := event.MessageID.String()

	// 解析 bot self ID（优先 config，否则从 event.SelfID 获取）
	selfID := strings.TrimSpace(cfg.BotQQ)
	if selfID == "" {
		selfID = strings.TrimSpace(event.SelfID.String())
	}
	// 尝试通过 get_login_info 自动获取并回写
	if selfID == "" || selfID == "0" {
		if obClient := c.buildOneBotClient(cfg); obClient != nil {
			if info, err := obClient.GetLoginInfo(ctx); err == nil && info != nil {
				got := strings.TrimSpace(info.UserID.String())
				if got != "" && got != "0" {
					selfID = got
					c.tryPersistBotQQ(ctx, ch.ID, got)
				}
			}
		}
	}

	mentionsBot := !isPrivate && event.MentionsQQ(selfID)

	// 精确回复检测：提取 reply 段的 message_id，通过 get_msg 判断是否回复了 bot
	isReplyToBot := false
	if !isPrivate {
		if replyMsgID := event.ReplyMessageID(); replyMsgID != "" {
			if obClient := c.buildOneBotClient(cfg); obClient != nil {
				if repliedMsg, err := obClient.GetMsg(ctx, replyMsgID); err == nil && repliedMsg != nil && repliedMsg.Sender != nil {
					repliedSender := strings.TrimSpace(repliedMsg.Sender.UserID.String())
					if repliedSender != "" && repliedSender == selfID {
						isReplyToBot = true
					}
				}
			}
		}
	}

	persona, personaRef, err := c.resolveQQPersona(ctx, ch)
	if err != nil {
		return err
	}

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

	// 群聊 group identity upsert（heartbeat 需要）
	var groupIdentity *data.ChannelIdentity
	if !isPrivate {
		gi, err := c.channelIdentitiesRepo.WithTx(tx).Upsert(ctx, "qq", platformChatID, nil, nil, nil)
		if err != nil {
			return err
		}
		groupIdentity = &gi
	}

	// 私聊 bind 访问控制
	if isPrivate {
		allowed, linkErr := qqPrivateChannelLinkAllowed(ctx, tx, ch.ID, identity, text, c.channelIdentityLinksRepo)
		if linkErr != nil {
			return linkErr
		}
		if !allowed {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			c.sendQQReply(ctx, ch, chatType, platformChatID, "当前账号未关联此接入。请使用 /bind 重新关联。")
			return nil
		}

		if handled, replyText, err := c.handleQQPrivateCommand(
			ctx, tx, &ch, identity, text,
		); err != nil {
			return err
		} else if handled {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			if replyText != "" {
				c.sendQQReply(ctx, ch, chatType, platformChatID, replyText)
			}
			return nil
		}
	}

	// 群聊命令处理
	if !isPrivate {
		if handled, replyText, cancelRunID, err := c.handleQQGroupCommand(
			ctx, tx, traceID, ch, cfg, identity, groupIdentity, text, platformChatID, platformMsgID,
		); err != nil {
			return err
		} else if handled {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			if cancelRunID != uuid.Nil {
				_, _ = c.pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgnotify.ChannelRunCancel, cancelRunID.String())
			}
			if replyText != "" {
				c.sendQQReply(ctx, ch, chatType, platformChatID, replyText)
			}
			return nil
		}

		// 群聊选择性回复判定
		shouldCreateRun := mentionsBot || isReplyToBot
		if !shouldCreateRun {
			slog.InfoContext(ctx, "qq_inbound_processed",
				"stage", "passive_persisted",
				"channel_id", ch.ID.String(),
				"platform_chat_id", platformChatID,
				"platform_message_id", platformMsgID,
				"mentions_bot", mentionsBot,
				"is_reply_to_bot", isReplyToBot,
			)
			if err := c.persistQQGroupPassiveMessage(ctx, tx, ch, identity, persona, chatType, platformChatID, platformMsgID, text, event.Time); err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
	}

	// --- active 路径：创建消息 + run ---

	if c.channelLedgerRepo != nil {
		ledgerMeta, _ := json.Marshal(map[string]any{
			"source":            "qq",
			"conversation_type": chatType,
			"mentions_bot":      mentionsBot,
			"is_reply_to_bot":   isReplyToBot,
		})
		if _, err := c.channelLedgerRepo.WithTx(tx).Record(ctx, data.ChannelMessageLedgerRecordInput{
			ChannelID:               ch.ID,
			ChannelType:             ch.ChannelType,
			Direction:               data.ChannelMessageDirectionInbound,
			PlatformConversationID:  platformChatID,
			PlatformMessageID:       platformMsgID,
			SenderChannelIdentityID: &identity.ID,
			MetadataJSON:            ledgerMeta,
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
		"mentions_bot":        mentionsBot,
		"is_reply_to_bot":     isReplyToBot,
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

// --- 命令处理 ---

func (c *qqConnector) handleQQPrivateCommand(
	ctx context.Context,
	tx pgx.Tx,
	channel *data.Channel,
	identity data.ChannelIdentity,
	text string,
) (bool, string, error) {
	if !strings.HasPrefix(text, "/") {
		return false, "", nil
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return false, "", nil
	}
	command := strings.TrimSpace(parts[0])
	switch {
	case command == "/help":
		return true, "/start — 查看连接状态\n/bind <code> — 绑定你的账号\n/new — 开启新会话\n/stop — 停止当前任务\n/help — 显示此帮助", nil
	case command == "/start":
		if len(parts) > 1 && strings.HasPrefix(parts[1], "bind_") {
			replyText, err := bindTelegramIdentity(ctx, tx, channel, identity, strings.TrimPrefix(parts[1], "bind_"),
				c.channelBindCodesRepo, c.channelIdentitiesRepo, c.channelIdentityLinksRepo, c.channelDMThreadsRepo, c.threadRepo)
			return true, replyText, err
		}
		return true, "已连接 Arkloop\n\n使用 /bind <code> 绑定账号\n私聊直接发消息开始对话，/new 开启新会话\n群内 @bot 触发对话，管理员可用 /new 重置会话", nil
	case command == "/bind":
		if len(parts) < 2 {
			return true, "用法：/bind <code>", nil
		}
		replyText, err := bindTelegramIdentity(ctx, tx, channel, identity, parts[1],
			c.channelBindCodesRepo, c.channelIdentitiesRepo, c.channelIdentityLinksRepo, c.channelDMThreadsRepo, c.threadRepo)
		return true, replyText, err
	case command == "/new":
		if channel == nil || channel.PersonaID == nil || *channel.PersonaID == uuid.Nil {
			return true, "当前会话未配置 persona。", nil
		}
		if err := c.channelDMThreadsRepo.WithTx(tx).DeleteByBinding(ctx, channel.ID, identity.ID, *channel.PersonaID); err != nil {
			return false, "", err
		}
		return true, "已开启新会话。", nil
	case command == "/stop":
		if channel == nil || channel.PersonaID == nil || *channel.PersonaID == uuid.Nil {
			return true, "当前没有运行中的任务。", nil
		}
		threadMap, err := c.channelDMThreadsRepo.WithTx(tx).GetByBinding(ctx, channel.ID, identity.ID, *channel.PersonaID)
		if err != nil {
			return false, "", err
		}
		if threadMap == nil {
			return true, "当前没有运行中的任务。", nil
		}
		activeRun, err := c.runEventRepo.GetActiveRootRunForThread(ctx, threadMap.ThreadID)
		if err != nil {
			return false, "", err
		}
		if activeRun == nil {
			return true, "当前没有运行中的任务。", nil
		}
		if _, err := c.runEventRepo.WithTx(tx).RequestCancel(ctx, activeRun.ID, identity.UserID, "", 0, nil); err != nil {
			return false, "", err
		}
		return true, "已请求停止当前任务。", nil
	default:
		return false, "", nil
	}
}

func (c *qqConnector) handleQQGroupCommand(
	ctx context.Context,
	tx pgx.Tx,
	traceID string,
	ch data.Channel,
	cfg qqChannelConfig,
	identity data.ChannelIdentity,
	groupIdentity *data.ChannelIdentity,
	text, platformChatID, platformMsgID string,
) (bool, string, uuid.UUID, error) {
	if !strings.HasPrefix(text, "/") {
		return false, "", uuid.Nil, nil
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return false, "", uuid.Nil, nil
	}
	command := strings.TrimSpace(parts[0])

	switch {
	case command == "/new":
		if ch.PersonaID == nil || *ch.PersonaID == uuid.Nil {
			return true, "当前会话未配置 persona。", uuid.Nil, nil
		}
		obClient := c.buildOneBotClient(cfg)
		userIDStr := identity.PlatformSubjectID
		if !qqIsGroupAdmin(ctx, obClient, platformChatID, userIDStr) {
			return true, "无权限。", uuid.Nil, nil
		}
		if c.channelGroupThreadsRepo != nil {
			if err := c.channelGroupThreadsRepo.WithTx(tx).DeleteByBinding(ctx, ch.ID, platformChatID, *ch.PersonaID); err != nil {
				return false, "", uuid.Nil, err
			}
		}
		return true, "已开启新会话。", uuid.Nil, nil

	case command == "/stop":
		if ch.PersonaID == nil || *ch.PersonaID == uuid.Nil {
			return true, "当前没有运行中的任务。", uuid.Nil, nil
		}
		obClient := c.buildOneBotClient(cfg)
		userIDStr := identity.PlatformSubjectID
		if !qqIsGroupAdmin(ctx, obClient, platformChatID, userIDStr) {
			return true, "无权限。", uuid.Nil, nil
		}
		if c.channelGroupThreadsRepo == nil {
			return true, "当前没有运行中的任务。", uuid.Nil, nil
		}
		threadMap, err := c.channelGroupThreadsRepo.WithTx(tx).GetByBinding(ctx, ch.ID, platformChatID, *ch.PersonaID)
		if err != nil {
			return false, "", uuid.Nil, err
		}
		if threadMap == nil {
			return true, "当前没有运行中的任务。", uuid.Nil, nil
		}
		activeRun, err := c.runEventRepo.GetActiveRootRunForThread(ctx, threadMap.ThreadID)
		if err != nil {
			return false, "", uuid.Nil, err
		}
		if activeRun == nil {
			return true, "当前没有运行中的任务。", uuid.Nil, nil
		}
		if _, err := c.runEventRepo.WithTx(tx).RequestCancel(ctx, activeRun.ID, identity.UserID, traceID, 0, nil); err != nil {
			return false, "", uuid.Nil, err
		}
		return true, "已请求停止当前任务。", activeRun.ID, nil

	case command == "/heartbeat" || strings.HasPrefix(command, "/heartbeat@"):
		heartbeatIdentity := identity
		if groupIdentity != nil {
			heartbeatIdentity = *groupIdentity
		}
		replyText, err := handleTelegramHeartbeatCommand(
			ctx, tx,
			ch.ID, ch.AccountID, ch.PersonaID,
			cfg.DefaultModel,
			heartbeatIdentity,
			text,
			c.channelIdentitiesRepo,
			c.personasRepo,
			c.entitlementSvc,
		)
		if err != nil {
			return false, "", uuid.Nil, err
		}
		return true, replyText, uuid.Nil, nil

	case command == "/help":
		return true, "/new — 开启新会话\n/stop — 停止当前任务\n/heartbeat — 心跳设置\n/help — 显示此帮助", uuid.Nil, nil

	default:
		return false, "", uuid.Nil, nil
	}
}

// --- passive persist ---

func (c *qqConnector) persistQQGroupPassiveMessage(
	ctx context.Context,
	tx pgx.Tx,
	ch data.Channel,
	identity data.ChannelIdentity,
	persona *data.Persona,
	chatType, platformChatID, platformMsgID, text string,
	unixTS int64,
) error {
	if persona == nil {
		return fmt.Errorf("qq passive ingest: persona required")
	}
	threadProjectID := derefUUID(persona.ProjectID)
	threadID, err := c.resolveQQThreadID(ctx, tx, ch, persona.ID, threadProjectID, identity, false, platformChatID)
	if err != nil {
		return err
	}

	if c.channelLedgerRepo != nil {
		ledgerMeta, _ := json.Marshal(map[string]any{
			"source":            "qq",
			"conversation_type": chatType,
		})
		if _, err := c.channelLedgerRepo.WithTx(tx).Record(ctx, data.ChannelMessageLedgerRecordInput{
			ChannelID:               ch.ID,
			ChannelType:             ch.ChannelType,
			Direction:               data.ChannelMessageDirectionInbound,
			PlatformConversationID:  platformChatID,
			PlatformMessageID:       platformMsgID,
			SenderChannelIdentityID: &identity.ID,
			MetadataJSON:            ledgerMeta,
		}); err != nil {
			return err
		}
	}

	displayName := identity.PlatformSubjectID
	if identity.DisplayName != nil && strings.TrimSpace(*identity.DisplayName) != "" {
		displayName = strings.TrimSpace(*identity.DisplayName)
	}

	projection := buildQQEnvelopeText(identity.ID, displayName, chatType, text, unixTS)
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
		"chat_type":           chatType,
	})

	if _, err := c.messageRepo.WithTx(tx).CreateStructuredWithMetadata(
		ctx, ch.AccountID, threadID, "user", projection, contentJSON, metadataJSON, identity.UserID,
	); err != nil {
		return err
	}
	return nil
}

// --- bind 访问控制 ---

func qqPrivateChannelLinkAllowed(
	ctx context.Context,
	tx pgx.Tx,
	channelID uuid.UUID,
	identity data.ChannelIdentity,
	commandText string,
	channelIdentityLinksRepo *data.ChannelIdentityLinksRepository,
) (bool, error) {
	if channelIdentityLinksRepo == nil || qqLinkBootstrapAllowed(commandText) {
		return true, nil
	}
	return channelIdentityLinksRepo.WithTx(tx).HasLink(ctx, channelID, identity.ID)
}

func qqLinkBootstrapAllowed(commandText string) bool {
	parts := strings.Fields(strings.TrimSpace(commandText))
	if len(parts) == 0 {
		return false
	}
	command := strings.TrimSpace(parts[0])
	return command == "/help" || command == "/bind" || command == "/start"
}

// --- OneBot 回复 ---

func (c *qqConnector) sendQQReply(ctx context.Context, ch data.Channel, chatType, target, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if client := c.buildOneBotClientFromChannel(ch); client != nil {
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if chatType == "group" {
			_, _ = client.SendGroupMsg(sendCtx, target, onebotclient.TextSegments(text))
		} else {
			_, _ = client.SendPrivateMsg(sendCtx, target, onebotclient.TextSegments(text))
		}
	}
}

// buildOneBotClient 从 config 构建 OneBot HTTP client
func (c *qqConnector) buildOneBotClient(cfg qqChannelConfig) *onebotclient.Client {
	httpURL := strings.TrimSpace(cfg.OneBotHTTPURL)
	if httpURL == "" {
		return nil
	}
	return onebotclient.NewClient(httpURL, strings.TrimSpace(cfg.OneBotToken), nil)
}

func (c *qqConnector) buildOneBotClientFromChannel(ch data.Channel) *onebotclient.Client {
	cfg, err := resolveQQChannelConfig(ch.ConfigJSON)
	if err != nil {
		return nil
	}
	return c.buildOneBotClient(cfg)
}

// tryPersistBotQQ 将自动获取的 bot QQ 号回写到 channel config
func (c *qqConnector) tryPersistBotQQ(ctx context.Context, channelID uuid.UUID, botQQ string) {
	if c.pool == nil || botQQ == "" {
		return
	}
	_, _ = c.pool.Exec(ctx,
		`UPDATE channels SET config_json = COALESCE(config_json, '{}'::jsonb) || jsonb_build_object('bot_qq', $1::text), updated_at = now() WHERE id = $2`,
		botQQ, channelID,
	)
}

// qqIsGroupAdmin 通过 get_group_member_info 检查用户是否为群管理员/群主
func qqIsGroupAdmin(ctx context.Context, client *onebotclient.Client, groupID, userID string) bool {
	if client == nil || groupID == "" || userID == "" {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := client.GetGroupMemberInfo(checkCtx, groupID, userID)
	if err != nil || info == nil {
		return false
	}
	return info.Role == "owner" || info.Role == "admin"
}

// --- 辅助函数 ---

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
	channelBindCodesRepo *data.ChannelBindCodesRepository,
	channelIdentityLinksRepo *data.ChannelIdentityLinksRepository,
	channelDMThreadsRepo *data.ChannelDMThreadsRepository,
	channelGroupThreadsRepo *data.ChannelGroupThreadsRepository,
	channelReceiptsRepo *data.ChannelMessageReceiptsRepository,
	personasRepo *data.PersonasRepository,
	threadRepo *data.ThreadRepository,
	messageRepo *data.MessageRepository,
	runEventRepo *data.RunEventRepository,
	jobRepo *data.JobRepository,
	entitlementSvc *entitlement.Service,
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
		channelsRepo:             channelsRepo,
		channelIdentitiesRepo:    channelIdentitiesRepo,
		channelBindCodesRepo:     channelBindCodesRepo,
		channelIdentityLinksRepo: channelIdentityLinksRepo,
		channelDMThreadsRepo:     channelDMThreadsRepo,
		channelGroupThreadsRepo:  channelGroupThreadsRepo,
		channelReceiptsRepo:      channelReceiptsRepo,
		channelLedgerRepo:        channelLedgerRepo,
		personasRepo:             personasRepo,
		threadRepo:               threadRepo,
		messageRepo:              messageRepo,
		runEventRepo:             runEventRepo,
		jobRepo:                  jobRepo,
		entitlementSvc:           entitlementSvc,
		pool:                     pool,
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

		ch := channels[0]
		if err := connector.HandleEvent(r.Context(), traceID, ch, event); err != nil {
			slog.ErrorContext(r.Context(), "qq_onebot_callback_error", "error", err, "channel_id", ch.ID)
			httpkit.WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
			return
		}

		httpkit.WriteJSON(w, traceID, nethttp.StatusOK, map[string]bool{"ok": true})
	}
}
