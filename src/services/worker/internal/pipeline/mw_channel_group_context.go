package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/routing"

	"github.com/google/uuid"
	"github.com/pkoukk/tiktoken-go"
)

const defaultChannelGroupMaxContextTokens = 32768

const groupTrimVisionTokensPerImage = 1024

const defaultGroupKeepImageTail = 10

var (
	groupTrimEncOnce sync.Once
	groupTrimEnc     *tiktoken.Tiktoken
	groupTrimEncErr  error
)

func groupTrimEncoder() *tiktoken.Tiktoken {
	groupTrimEncOnce.Do(func() {
		groupTrimEnc, groupTrimEncErr = tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	})
	if groupTrimEncErr != nil {
		return nil
	}
	return groupTrimEnc
}

// GroupContextTrimDeps 群聊投影与预算裁剪所需的依赖。
type GroupContextTrimDeps struct {
	Pool            CompactPersistDB
	MessagesRepo    data.MessagesRepository
	EventsRepo      CompactRunEventAppender
	EmitDebugEvents bool
}

// NewChannelGroupContextTrimMiddleware 在 Routing 之后运行，只负责群聊 envelope 投影和预算裁剪。
// 真正的 compact 统一由 NewContextCompactMiddleware 处理。
func NewChannelGroupContextTrimMiddleware(deps ...GroupContextTrimDeps) RunMiddleware {
	keepImageTail := defaultGroupKeepImageTail
	if raw := strings.TrimSpace(os.Getenv("ARKLOOP_CHANNEL_GROUP_KEEP_IMAGE_TAIL")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			keepImageTail = n
		}
	}

	var dep GroupContextTrimDeps
	if len(deps) > 0 {
		dep = deps[0]
	}

	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc == nil || rc.ChannelContext == nil {
			return next(ctx, rc)
		}

		projected, skipped := projectGroupEnvelopes(rc)
		if skipped > 0 {
			slog.WarnContext(ctx, "envelope_projection", "projected", projected, "skipped", skipped, "run_id", rc.Run.ID.String())
		}

		if !IsTelegramGroupLikeConversation(rc.ChannelContext.ConversationType) {
			return next(ctx, rc)
		}

		maxTokens := resolveGroupMaxTokens(rc)

		stripOlderImages(rc, keepImageTail)
		beforeTrim := snapshotGroupTrimStats(rc)
		trimRunContextMessagesToApproxTokens(rc, maxTokens)
		trimEvent := buildGroupTrimEvent(beforeTrim, snapshotGroupTrimStats(rc), maxTokens, false)

		nextErr := next(ctx, rc)

		if trimEvent != nil && dep.EmitDebugEvents {
			postCtx, cancel := context.WithTimeout(context.Background(), contextCompactPostWriteTimeout)
			defer cancel()
			if err := appendContextCompactRunEvent(postCtx, dep.Pool, dep.EventsRepo, rc, trimEvent); err != nil {
				slog.WarnContext(ctx, "group_trim", "phase", "run_event", "err", err.Error(), "run_id", rc.Run.ID.String())
			}
		}

		return nextErr
	}
}

// resolveGroupCompactTriggerTokens 为群聊复用通用 compact 触发配置。
// 优先使用 route context window + context.compact.*；
// 若当前 run 未注入配置，再回退到环境变量/硬编码，避免测试与旧路径失效。
func resolveGroupCompactTriggerTokens(rc *RunContext) (int, int) {
	if rc != nil {
		window := 0
		if rc.SelectedRoute != nil {
			window = routing.RouteContextWindowTokens(rc.SelectedRoute.Route)
		}
		trigger, resolvedWindow := compactPersistTriggerTokens(rc.ContextCompact, window)
		if trigger > 0 {
			return trigger, resolvedWindow
		}
	}
	if raw := strings.TrimSpace(os.Getenv("ARKLOOP_CHANNEL_GROUP_MAX_CONTEXT_TOKENS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n, n
		}
	}
	return defaultChannelGroupMaxContextTokens, defaultChannelGroupMaxContextTokens
}

// resolveGroupMaxTokens 返回群聊 trim / compact 共享的触发预算。
func resolveGroupMaxTokens(rc *RunContext) int {
	trigger, _ := resolveGroupCompactTriggerTokens(rc)
	return trigger
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

type groupTrimStats struct {
	MessageCount         int
	RealMessageCount     int
	HasReplacementPrefix bool
	EstimatedTrimWeight  int
	EstimatedTextTokens  int
	EstimatedImageTokens int
}

func snapshotGroupTrimStats(rc *RunContext) groupTrimStats {
	if rc == nil || len(rc.Messages) == 0 {
		return groupTrimStats{}
	}
	msgs := rc.Messages
	ids := rc.ThreadMessageIDs
	fixedPrefix, _ := leadingCompactPrefixTokenCount(msgs, ids)
	stats := groupTrimStats{
		MessageCount:         len(msgs),
		RealMessageCount:     len(msgs) - fixedPrefix,
		HasReplacementPrefix: fixedPrefix > 0,
	}
	for i := range msgs {
		stats.EstimatedTrimWeight += messageTokens(&msgs[i])
		stats.EstimatedTextTokens += approxLLMMessageTextTokens(msgs[i])
		stats.EstimatedImageTokens += approxLLMMessageImageTokens(msgs[i])
	}
	return stats
}

func buildGroupTrimEvent(before, after groupTrimStats, maxTokens int, compactTriggered bool) map[string]any {
	if before.RealMessageCount <= after.RealMessageCount &&
		before.EstimatedTrimWeight == after.EstimatedTrimWeight {
		return nil
	}
	droppedCount := before.RealMessageCount - after.RealMessageCount
	if droppedCount < 0 {
		droppedCount = 0
	}
	return map[string]any{
		"op":                            "group_trim",
		"phase":                         "completed",
		"max_tokens":                    maxTokens,
		"messages_before":               before.MessageCount,
		"messages_after":                after.MessageCount,
		"kept_count":                    after.RealMessageCount,
		"dropped_count":                 droppedCount,
		"has_replacement_prefix":        before.HasReplacementPrefix,
		"compact_triggered":             compactTriggered,
		"estimated_trim_weight_before":  before.EstimatedTrimWeight,
		"estimated_trim_weight_after":   after.EstimatedTrimWeight,
		"estimated_text_tokens_before":  before.EstimatedTextTokens,
		"estimated_text_tokens_after":   after.EstimatedTextTokens,
		"estimated_image_tokens_before": before.EstimatedImageTokens,
		"estimated_image_tokens_after":  after.EstimatedImageTokens,
	}
}

// trimRunContextMessagesToApproxTokens 会优先保留头部 replacement，再从尾部保留真实消息。
func trimRunContextMessagesToApproxTokens(rc *RunContext, maxTokens int) {
	if rc == nil || maxTokens <= 0 || len(rc.Messages) == 0 {
		return
	}
	msgs := rc.Messages
	ids := rc.ThreadMessageIDs
	alignedIDs := len(ids) == len(msgs)

	fixedPrefix, snapshotTokens := leadingCompactPrefixTokenCount(msgs, ids)
	hasReplacement := fixedPrefix > 0
	realStart := fixedPrefix

	budget := maxTokens - snapshotTokens
	if budget <= 0 {
		if hasReplacement && keepLatestRealMessageWithoutPrefix(rc, msgs, ids, alignedIDs, maxTokens) {
			return
		}
		return
	}

	realMsgs := msgs[realStart:]
	total := 0
	start := len(realMsgs)
	for i := len(realMsgs) - 1; i >= 0; i-- {
		t := messageTokens(&realMsgs[i])
		if total+t > budget {
			break
		}
		total += t
		start = i
	}

	start = ensureToolPairIntegrity(realMsgs, start)
	if len(realMsgs) > 0 && start >= len(realMsgs) {
		start = len(realMsgs) - 1
	}

	if start <= 0 && !hasReplacement {
		return
	}

	if hasReplacement {
		if len(realMsgs) > 0 && start >= len(realMsgs) {
			if keepLatestRealMessageWithoutPrefix(rc, msgs, ids, alignedIDs, maxTokens) {
				return
			}
		}
		kept := realMsgs[start:]
		if len(kept) == 0 && keepLatestRealMessageWithoutPrefix(rc, msgs, ids, alignedIDs, maxTokens) {
			return
		}
		rc.Messages = make([]llm.Message, 0, fixedPrefix+len(kept))
		rc.Messages = append(rc.Messages, msgs[:fixedPrefix]...)
		rc.Messages = append(rc.Messages, kept...)
		if alignedIDs {
			keptIDs := ids[realStart+start:]
			rc.ThreadMessageIDs = make([]uuid.UUID, 0, fixedPrefix+len(keptIDs))
			rc.ThreadMessageIDs = append(rc.ThreadMessageIDs, ids[:fixedPrefix]...)
			rc.ThreadMessageIDs = append(rc.ThreadMessageIDs, keptIDs...)
		}
	} else {
		if start >= len(realMsgs) {
			return
		}
		rc.Messages = msgs[start:]
		if alignedIDs {
			rc.ThreadMessageIDs = ids[start:]
		}
	}
}

func keepLatestRealMessageWithoutPrefix(rc *RunContext, msgs []llm.Message, ids []uuid.UUID, alignedIDs bool, maxTokens int) bool {
	if len(msgs) == 0 {
		return false
	}
	realMsgs := msgs
	realIDs := ids
	fixedPrefix := leadingCompactPrefixMessageCount(msgs, ids)
	if fixedPrefix > 0 {
		if len(msgs) <= fixedPrefix {
			return false
		}
		realMsgs = msgs[fixedPrefix:]
		if alignedIDs {
			realIDs = ids[fixedPrefix:]
		}
	}
	if len(realMsgs) == 0 {
		return false
	}

	total := 0
	start := len(realMsgs)
	for i := len(realMsgs) - 1; i >= 0; i-- {
		t := messageTokens(&realMsgs[i])
		if total+t > maxTokens && i < len(realMsgs)-1 {
			break
		}
		total += t
		start = i
	}
	start = ensureToolPairIntegrity(realMsgs, start)
	if start >= len(realMsgs) {
		start = len(realMsgs) - 1
	}
	rc.Messages = append([]llm.Message(nil), realMsgs[start:]...)
	if alignedIDs {
		rc.ThreadMessageIDs = append([]uuid.UUID(nil), realIDs[start:]...)
	}
	return true
}

func leadingCompactPrefixTokenCount(msgs []llm.Message, ids []uuid.UUID) (int, int) {
	count := leadingCompactPrefixMessageCount(msgs, ids)
	total := 0
	for i := 0; i < count; i++ {
		total += messageTokens(&msgs[i])
	}
	return count, total
}

// messageTokens 估算单条在历史截断里的权重，顺序：
// 1) assistant 且 usage_records.output_tokens>0（模型侧真实 completion，Desktop 与 Postgres 的 ListByThread 均已 JOIN）
// 2) tiktoken o200k 估正文+tool；图按固定 vision 预算
// 3) tiktoken 初始化失败则 legacy：rune/3
//
// user 等角色不能用 output_tokens：metadata 里同 run_id 会 JOIN 到同一条 usage，数值语义不是「本条 user 长度」。
func messageTokens(m *llm.Message) int {
	if m != nil && strings.EqualFold(strings.TrimSpace(m.Role), "assistant") &&
		m.OutputTokens != nil && *m.OutputTokens > 0 {
		return int(*m.OutputTokens)
	}
	if m == nil {
		return 1
	}
	return approxLLMMessageTokens(*m)
}

func approxLLMMessageTokens(m llm.Message) int {
	enc := groupTrimEncoder()
	if enc == nil {
		return approxLLMMessageTokensLegacy(m)
	}
	n := approxLLMMessageTextTokensWithEncoder(enc, m)
	n += approxLLMMessageImageTokens(m)
	if n < 1 {
		return 1
	}
	return n
}

func approxLLMMessageTextTokens(m llm.Message) int {
	enc := groupTrimEncoder()
	if enc == nil {
		return approxLLMMessageTextTokensLegacy(m)
	}
	return approxLLMMessageTextTokensWithEncoder(enc, m)
}

func approxLLMMessageTextTokensWithEncoder(enc *tiktoken.Tiktoken, m llm.Message) int {
	const tokensPerMessage = 3
	n := tokensPerMessage
	n += len(enc.Encode(m.Role, nil, nil))
	body := messageText(m)
	for _, tc := range m.ToolCalls {
		body += "\n"
		body += tc.ToolName
		if b, err := json.Marshal(tc.ArgumentsJSON); err == nil {
			body += string(b)
		}
	}
	n += len(enc.Encode(body, nil, nil))
	if n < 1 {
		return 1
	}
	return n
}

func approxLLMMessageImageTokens(m llm.Message) int {
	total := 0
	for _, p := range m.Content {
		if p.Kind() == messagecontent.PartTypeImage {
			total += groupTrimVisionTokensPerImage
		}
	}
	return total
}

func approxLLMMessageTokensLegacy(m llm.Message) int {
	n := approxLLMMessageTextTokensLegacy(m)
	n += approxLLMMessageImageTokensLegacy(m)
	if n < 1 {
		return 1
	}
	return n
}

func approxLLMMessageTextTokensLegacy(m llm.Message) int {
	n := 0
	for _, p := range m.Content {
		n += utf8.RuneCountInString(p.Text)
		n += utf8.RuneCountInString(p.ExtractedText)
		if p.Attachment != nil {
			n += 64
		}
		if len(p.Data) > 0 && p.Kind() != messagecontent.PartTypeImage {
			raw := len(p.Data) / 4
			n += raw
		}
	}
	for _, tc := range m.ToolCalls {
		n += utf8.RuneCountInString(tc.ToolName)
		if b, err := json.Marshal(tc.ArgumentsJSON); err == nil {
			n += len(b) / 4
		}
	}
	out := n / 3
	if out < 1 {
		return 1
	}
	return out
}

func approxLLMMessageImageTokensLegacy(m llm.Message) int {
	total := 0
	for _, p := range m.Content {
		if p.Kind() != messagecontent.PartTypeImage {
			continue
		}
		raw := len(p.Data) / 4
		if raw > 3072 {
			raw = 3072
		}
		total += raw / 3
		if total < 1 {
			total = 1
		}
	}
	return total
}
