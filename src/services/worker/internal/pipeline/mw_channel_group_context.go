package pipeline

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/imageutil"
	"arkloop/services/worker/internal/llm"
)

const defaultGroupKeepImageTail = 10

// GroupContextTrimDeps 群聊投影与预算裁剪所需的依赖。
type GroupContextTrimDeps struct {
	Pool            CompactPersistDB
	MessagesRepo    data.MessagesRepository
	EventsRepo      CompactRunEventAppender
	EmitDebugEvents bool
	AttachmentStore MessageAttachmentStore
}

// NewChannelGroupContextTrimMiddleware 在 Routing 之后运行，只负责群聊 envelope 投影和图片瘦身。
// 历史压缩统一交给 replacement compact 主路径处理，这里不再直接裁掉消息前缀。
func NewChannelGroupContextTrimMiddleware(deps ...GroupContextTrimDeps) RunMiddleware {
	keepImageTail := defaultGroupKeepImageTail
	cfg := GroupContextTrimDeps{}
	if len(deps) > 0 {
		cfg = deps[0]
	}
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
		messages, err := materializeMessageImages(ctx, cfg.AttachmentStore, rc.Messages)
		if err != nil {
			return err
		}
		rc.Messages = messages
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

func materializeMessageImages(ctx context.Context, store MessageAttachmentStore, msgs []llm.Message) ([]llm.Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		parts := out[i].Content
		partsCopied := false
		for j := range parts {
			if parts[j].Kind() != messagecontent.PartTypeImage || len(parts[j].Data) > 0 {
				continue
			}
			if parts[j].Attachment == nil || strings.TrimSpace(parts[j].Attachment.Key) == "" {
				return nil, fmt.Errorf("message image attachment is required")
			}
			if store == nil {
				return nil, fmt.Errorf("message attachment store not configured")
			}
			if !partsCopied {
				parts = append([]llm.ContentPart(nil), out[i].Content...)
				partsCopied = true
			}
			dataBytes, contentType, err := store.GetWithContentType(ctx, parts[j].Attachment.Key)
			if err != nil {
				if objectstore.IsNotFound(err) {
					return nil, fmt.Errorf("message attachment not found")
				}
				return nil, err
			}
			attachment := *parts[j].Attachment
			if strings.TrimSpace(contentType) != "" {
				attachment.MimeType = strings.TrimSpace(contentType)
			}
			dataBytes, attachment.MimeType = imageutil.ProcessImage(dataBytes, attachment.MimeType)
			parts[j].Attachment = &attachment
			parts[j].Data = dataBytes
		}
		if partsCopied {
			out[i].Content = parts
		}
	}
	return out, nil
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
