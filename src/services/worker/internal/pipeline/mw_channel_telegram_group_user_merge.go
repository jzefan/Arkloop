package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/llm"

	"github.com/google/uuid"
)

// NewChannelTelegramGroupUserMergeMiddleware 将 Telegram 群聊线程尾部、自最后一条 assistant 起的连续多条 user
// 合并为单条 user 再交给后续中间件与 LLM。入库仍为每人一条；合并后 ThreadMessageIDs 仅保留尾段最后一条 user 的 id，
// 中间几条 id 不再出现在数组中（与 context compact 的 id 对齐语义一致）。
func NewChannelTelegramGroupUserMergeMiddleware() RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		_ = ctx
		if rc == nil || rc.ChannelContext == nil {
			return next(ctx, rc)
		}
		if strings.ToLower(strings.TrimSpace(rc.ChannelContext.ChannelType)) != "telegram" {
			return next(ctx, rc)
		}
		if !IsTelegramGroupLikeConversation(rc.ChannelContext.ConversationType) {
			return next(ctx, rc)
		}
		msgs, ids := mergeTelegramGroupTrailingUserBurst(rc.Messages, rc.ThreadMessageIDs)
		rc.Messages = msgs
		rc.ThreadMessageIDs = ids
		return next(ctx, rc)
	}
}

func mergeTelegramGroupTrailingUserBurst(msgs []llm.Message, ids []uuid.UUID) ([]llm.Message, []uuid.UUID) {
	if len(msgs) != len(ids) || len(msgs) < 2 {
		return msgs, ids
	}
	lastAsst := -1
	for i := range msgs {
		if strings.EqualFold(strings.TrimSpace(msgs[i].Role), "assistant") {
			lastAsst = i
		}
	}
	tailStart := lastAsst + 1
	tail := msgs[tailStart:]
	tailIDs := ids[tailStart:]
	if len(tail) < 2 {
		return msgs, ids
	}
	for _, m := range tail {
		if !strings.EqualFold(strings.TrimSpace(m.Role), "user") {
			return msgs, ids
		}
		if len(m.ToolCalls) > 0 {
			return msgs, ids
		}
	}
	mergedContent := mergeUserBurstContent(tail)
	merged := llm.Message{
		Role:    "user",
		Content: mergedContent,
	}
	outMsgs := make([]llm.Message, 0, len(msgs)-len(tail)+1)
	outMsgs = append(outMsgs, msgs[:tailStart]...)
	outMsgs = append(outMsgs, merged)
	outIDs := make([]uuid.UUID, 0, len(ids)-len(tail)+1)
	outIDs = append(outIDs, ids[:tailStart]...)
	outIDs = append(outIDs, tailIDs[len(tailIDs)-1])
	return outMsgs, outIDs
}

func mergeUserBurstContent(tail []llm.Message) []llm.ContentPart {
	if compacted, ok := compactTelegramGroupEnvelopeBurst(tail); ok {
		return []llm.ContentPart{{Type: messagecontent.PartTypeText, Text: compacted}}
	}
	if mergedText, ok := mergePureTextBurst(tail); ok {
		return []llm.ContentPart{{Type: messagecontent.PartTypeText, Text: mergedText}}
	}
	const sep = "\n\n"
	var parts []llm.ContentPart
	for i := range tail {
		if i > 0 {
			parts = append(parts, llm.ContentPart{Type: messagecontent.PartTypeText, Text: sep})
		}
		for _, p := range tail[i].Content {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return []llm.ContentPart{{Type: messagecontent.PartTypeText, Text: ""}}
	}
	return parts
}

type telegramEnvelopeMessage struct {
	meta map[string]string
	body string
}

func compactTelegramGroupEnvelopeBurst(tail []llm.Message) (string, bool) {
	if len(tail) < 2 {
		return "", false
	}
	items := make([]telegramEnvelopeMessage, 0, len(tail))
	for _, msg := range tail {
		text, ok := singleTextMessage(msg)
		if !ok {
			return "", false
		}
		meta, body, ok := parseTelegramEnvelopeText(text)
		if !ok {
			return "", false
		}
		if !strings.EqualFold(strings.TrimSpace(meta["channel"]), "telegram") {
			return "", false
		}
		body = compactTelegramEnvelopeBody(meta, body)
		if strings.TrimSpace(body) == "" {
			return "", false
		}
		items = append(items, telegramEnvelopeMessage{meta: meta, body: body})
	}

	channel := commonEnvelopeValue(items, "channel")
	conversationType := commonEnvelopeValue(items, "conversation-type")
	if channel == "" || conversationType == "" {
		return "", false
	}
	conversationTitle := commonEnvelopeValue(items, "conversation-title")
	messageThreadID := commonEnvelopeValue(items, "message-thread-id")

	nameRefs := map[string]map[string]struct{}{}
	for _, item := range items {
		name := strings.TrimSpace(item.meta["display-name"])
		ref := strings.TrimSpace(item.meta["sender-ref"])
		if name == "" {
			continue
		}
		bucket := nameRefs[name]
		if bucket == nil {
			bucket = map[string]struct{}{}
			nameRefs[name] = bucket
		}
		bucket[ref] = struct{}{}
	}

	lines := []string{
		fmt.Sprintf(`channel: %q`, channel),
		fmt.Sprintf(`conversation-type: %q`, conversationType),
	}
	if conversationTitle != "" {
		lines = append(lines, fmt.Sprintf(`conversation-title: %q`, conversationTitle))
	}
	if messageThreadID != "" {
		lines = append(lines, fmt.Sprintf(`message-thread-id: %q`, messageThreadID))
	}

	var bodyLines []string
	for _, item := range items {
		name := strings.TrimSpace(item.meta["display-name"])
		duplicateDisplay := false
		if refs := nameRefs[name]; len(refs) > 1 {
			duplicateDisplay = true
		}
		speaker := compactTelegramBurstSpeaker(item.meta, duplicateDisplay)
		ts := compactTelegramBurstTime(item.meta["time"])
		bodyLines = append(bodyLines, renderCompactTelegramBurstLine(ts, speaker, item.body))
	}

	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + strings.Join(bodyLines, "\n"), true
}

func mergePureTextBurst(tail []llm.Message) (string, bool) {
	if len(tail) == 0 {
		return "", false
	}
	texts := make([]string, 0, len(tail))
	for _, msg := range tail {
		text, ok := singleTextMessage(msg)
		if !ok {
			return "", false
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n\n"), true
}

func singleTextMessage(msg llm.Message) (string, bool) {
	if len(msg.Content) == 0 {
		return "", false
	}
	var sb strings.Builder
	for _, part := range msg.Content {
		if !strings.EqualFold(strings.TrimSpace(part.Type), messagecontent.PartTypeText) {
			return "", false
		}
		sb.WriteString(part.Text)
	}
	return sb.String(), true
}

func parseTelegramEnvelopeText(text string) (map[string]string, string, bool) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(normalized, "---\n"), "\n---\n", 2)
	if len(parts) != 2 {
		return nil, "", false
	}
	meta := map[string]string{}
	for _, line := range strings.Split(parts[0], "\n") {
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key == "" || value == "" {
			continue
		}
		meta[key] = strings.Trim(value, `"`)
	}
	body := strings.TrimSpace(parts[1])
	if len(meta) == 0 || body == "" {
		return nil, "", false
	}
	return meta, body, true
}

func commonEnvelopeValue(items []telegramEnvelopeMessage, key string) string {
	if len(items) == 0 {
		return ""
	}
	first := strings.TrimSpace(items[0].meta[key])
	if first == "" {
		return ""
	}
	for _, item := range items[1:] {
		if strings.TrimSpace(item.meta[key]) != first {
			return ""
		}
	}
	return first
}

func compactTelegramEnvelopeBody(meta map[string]string, body string) string {
	cleaned := strings.TrimSpace(body)
	title := strings.TrimSpace(meta["conversation-title"])
	if title != "" {
		prefix := "[Telegram in " + title + "]"
		if strings.HasPrefix(cleaned, prefix) {
			cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, prefix))
		}
	}
	if strings.HasPrefix(cleaned, "[Telegram]") {
		cleaned = strings.TrimSpace(strings.TrimPrefix(cleaned, "[Telegram]"))
	}
	return cleaned
}

func compactTelegramBurstSpeaker(meta map[string]string, duplicateDisplay bool) string {
	displayName := strings.TrimSpace(meta["display-name"])
	shortRef := compactTelegramSenderRef(meta["sender-ref"])
	switch {
	case displayName == "" && shortRef == "":
		return "user"
	case displayName == "":
		return shortRef
	case duplicateDisplay && shortRef != "":
		return displayName + " <" + shortRef + ">"
	default:
		return displayName
	}
}

func compactTelegramSenderRef(ref string) string {
	cleaned := strings.TrimSpace(ref)
	if cleaned == "" {
		return ""
	}
	if len(cleaned) > 8 {
		return cleaned[:8]
	}
	return cleaned
}

func compactTelegramBurstTime(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return "time?"
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, cleaned); err == nil {
			return parsed.UTC().Format("15:04:05")
		}
	}
	return cleaned
}

func renderCompactTelegramBurstLine(ts, speaker, body string) string {
	text := strings.TrimSpace(body)
	if text == "" {
		return fmt.Sprintf("[%s] %s", ts, speaker)
	}
	lines := strings.Split(text, "\n")
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(ts)
	sb.WriteString("] ")
	sb.WriteString(strings.TrimSpace(speaker))
	sb.WriteString(": ")
	sb.WriteString(strings.TrimSpace(lines[0]))
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		sb.WriteString("\n  ")
		sb.WriteString(trimmed)
	}
	return sb.String()
}
