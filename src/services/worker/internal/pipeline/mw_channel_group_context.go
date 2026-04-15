package pipeline

import (
	"context"
	"os"
	"strconv"
	"strings"

	"arkloop/services/worker/internal/data"
)

const defaultGroupKeepImageTail = 10

// GroupContextTrimDeps 群聊投影与预算裁剪所需的依赖。
type GroupContextTrimDeps struct {
	Pool            CompactPersistDB
	MessagesRepo    data.MessagesRepository
	EventsRepo      CompactRunEventAppender
	EmitDebugEvents bool
}

// NewChannelGroupContextTrimMiddleware 在 Routing 之后运行，只负责群聊 envelope 投影和图片瘦身。
// 历史压缩统一交给 replacement compact 主路径处理，这里不再直接裁掉消息前缀。
func NewChannelGroupContextTrimMiddleware(deps ...GroupContextTrimDeps) RunMiddleware {
	keepImageTail := defaultGroupKeepImageTail
	if raw := strings.TrimSpace(os.Getenv("ARKLOOP_CHANNEL_GROUP_KEEP_IMAGE_TAIL")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			keepImageTail = n
		}
	}

	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc == nil || rc.ChannelContext == nil {
			return next(ctx, rc)
		}

		projectGroupEnvelopes(rc)

		if !IsTelegramGroupLikeConversation(rc.ChannelContext.ConversationType) {
			return next(ctx, rc)
		}

		stripOlderImages(rc, keepImageTail)
		return next(ctx, rc)
	}
}

// stripOlderImages 将更早的 image part 替换为带 attachment_key 的占位符，仅保留最近 keepImages 个真实图片。
func stripOlderImages(rc *RunContext, keepImages int) {
	if rc == nil || len(rc.Messages) == 0 || keepImages < 0 {
		return
	}
	rewritten, _ := stripOlderImagePartsKeepingTail(rc.Messages, keepImages)
	if len(rewritten) == 0 {
		return
	}
	rc.Messages = rewritten
}

// IsTelegramGroupLikeConversation 判断 Telegram 侧群 / 超级群 / 频道（非私信）。
func IsTelegramGroupLikeConversation(ct string) bool {
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "group", "supergroup", "channel":
		return true
	default:
		return false
	}
}
