package pipeline

import (
	"encoding/json"
	"strings"
)

// ResolveTelegramOutboundReplyTo 用于 sendMessage 的 reply_to_message_id；私聊默认不引用。
func ResolveTelegramOutboundReplyTo(rc *RunContext, ux TelegramChannelUX) *ChannelMessageRef {
	if rc == nil || rc.ChannelContext == nil || rc.ChannelContext.TriggerMessage == nil {
		return nil
	}
	if !ux.QuoteInboundForConversation(rc.ChannelContext.ConversationType) {
		return nil
	}
	return rc.ChannelContext.TriggerMessage
}

// TelegramChannelUX is parsed from channels.config_json (telegram-specific UX flags).
type TelegramChannelUX struct {
	TypingIndicator bool
	ReactionEmoji   string
	// QuoteInboundMessage nil 时：私聊不引用用户消息，其它会话类型引用（reply_to_message_id）。
	QuoteInboundMessage *bool
}

// ParseTelegramChannelUX reads optional keys:
//   - telegram_typing_indicator (bool, default true if absent)
//   - telegram_reaction_emoji (string, empty = off)
//   - telegram_quote_inbound_message (bool, default：私聊 false，其它 true)
func ParseTelegramChannelUX(configJSON []byte) TelegramChannelUX {
	out := TelegramChannelUX{TypingIndicator: true}
	if len(configJSON) == 0 {
		return out
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(configJSON, &m); err != nil {
		return out
	}
	if raw, ok := m["telegram_typing_indicator"]; ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err == nil {
			out.TypingIndicator = v
		}
	}
	if raw, ok := m["telegram_reaction_emoji"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			out.ReactionEmoji = strings.TrimSpace(s)
		}
	}
	if raw, ok := m["telegram_quote_inbound_message"]; ok {
		var v bool
		if err := json.Unmarshal(raw, &v); err == nil {
			vv := v
			out.QuoteInboundMessage = &vv
		}
	}
	return out
}

// QuoteInboundForConversation 决定是否对发往 Telegram 的消息使用 reply_to（引用条）。
func (ux TelegramChannelUX) QuoteInboundForConversation(conversationType string) bool {
	if ux.QuoteInboundMessage != nil {
		return *ux.QuoteInboundMessage
	}
	return !strings.EqualFold(strings.TrimSpace(conversationType), "private")
}
