package tools

import "github.com/google/uuid"

// ChannelToolSurface 供 Channel 相关工具读取；不含密钥，chat 仅来自投递载荷不可由模型覆盖。
type ChannelToolSurface struct {
	ChannelID        uuid.UUID
	ChannelType      string
	PlatformChatID   string
	InboundMessageID string
	MessageThreadID  *string
}
