package pipeline

import (
	"context"
	"log/slog"
	"strings"

	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
)

// NewHeartbeatScheduleMiddleware 在 run 结束后 upsert scheduled_triggers。
// interval/model 从 channel_identities 读取（由 /heartbeat 命令设置）。
// heartbeat run 本身不执行（避免无限循环）。
func NewHeartbeatScheduleMiddleware(db data.DB) RunMiddleware {
	repo := data.ScheduledTriggersRepository{}
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		err := next(ctx, rc)

		if err != nil || rc == nil || db == nil {
			return err
		}
		if rc.LLMHeartbeatRun {
			return nil
		}
		def := rc.PersonaDefinition
		if def == nil || !def.HeartbeatEnabled {
			return nil
		}
		if rc.ChannelContext == nil || !IsTelegramGroupLikeConversation(rc.ChannelContext.ConversationType) {
			return nil
		}

		identityID := rc.ChannelContext.SenderChannelIdentityID
		if identityID == uuid.Nil {
			slog.WarnContext(ctx, "heartbeat_schedule: no sender channel identity id, skipping")
			return nil
		}

		// 读 channel_identities 的 heartbeat 配置（interval/model 由用户通过 /heartbeat 命令设置）
		iv := 30
		model := ""
		cfg, cfgErr := data.GetChannelIdentityHeartbeatConfig(ctx, db, identityID)
		if cfgErr != nil {
			slog.WarnContext(ctx, "heartbeat_schedule: get channel identity config failed", "error", cfgErr)
		} else if cfg != nil {
			if cfg.IntervalMinutes > 0 {
				iv = cfg.IntervalMinutes
			}
			model = cfg.Model
		}

		// model fallback：InputJSON → PersonaDefinition
		if strings.TrimSpace(model) == "" {
			if m, ok := rc.InputJSON["model"].(string); ok && strings.TrimSpace(m) != "" {
				model = strings.TrimSpace(m)
			}
		}
		if strings.TrimSpace(model) == "" && def.Model != nil {
			model = strings.TrimSpace(*def.Model)
		}

		if upsertErr := repo.UpsertHeartbeat(ctx, db, rc.Run.AccountID, identityID, def.ID, model, iv); upsertErr != nil {
			slog.WarnContext(ctx, "heartbeat schedule upsert failed", "identity_id", identityID, "error", upsertErr)
		} else {
			slog.InfoContext(ctx, "heartbeat schedule upserted", "identity_id", identityID, "interval_min", iv, "model", model)
		}
		return nil
	}
}
