package pipeline

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"arkloop/services/shared/weixinclient"
)

const weixinMessageMaxLen = 2048

// WeixinChannelSender 通过微信 iLink API 发送消息。
type WeixinChannelSender struct {
	client       *weixinclient.Client
	segmentDelay time.Duration
}

// NewWeixinChannelSender 使用环境变量创建微信渠道发送器。
func NewWeixinChannelSender(baseURL, token string) *WeixinChannelSender {
	return NewWeixinChannelSenderWithClient(weixinclient.NewClient(baseURL, token, nil), resolveSegmentDelay())
}

// NewWeixinChannelSenderWithClient 用于测试注入。
func NewWeixinChannelSenderWithClient(client *weixinclient.Client, segmentDelay time.Duration) *WeixinChannelSender {
	return &WeixinChannelSender{
		client:       client,
		segmentDelay: segmentDelay,
	}
}

// SendText 发送文本消息到微信用户。
// - 按 weixinMessageMaxLen 分片
// - 第一片带 ContextToken（从 target.Metadata["context_token"] 取）
// - 段间有 segmentDelay 延迟
func (s *WeixinChannelSender) SendText(ctx context.Context, target ChannelDeliveryTarget, text string) ([]string, error) {
	toUserID := target.Conversation.Target
	contextToken := ""
	if target.Metadata != nil {
		if ct, ok := target.Metadata["context_token"].(string); ok {
			contextToken = strings.TrimSpace(ct)
		}
	}

	segments := splitWeixinMessage(text, weixinMessageMaxLen)
	ids := make([]string, 0, len(segments))
	for idx, seg := range segments {
		req := weixinclient.SendMessageRequest{
			ToUserID:     toUserID,
			MessageType:  2,
			MessageState: 2,
			ItemList: []weixinclient.MessageItem{
				{Type: 1, TextItem: &weixinclient.TextItem{Text: seg}},
			},
		}
		if idx == 0 && contextToken != "" {
			req.ContextToken = contextToken
		}

		resp, err := s.client.SendMessage(ctx, &req)
		if err != nil {
			return ids, err
		}
		if resp != nil && resp.MessageID != "" {
			ids = append(ids, resp.MessageID)
		}
		if idx < len(segments)-1 && s.segmentDelay > 0 {
			time.Sleep(s.segmentDelay)
		}
	}
	return ids, nil
}

// splitWeixinMessage 按字符数拆分长消息，优先在自然断点处断开。
func splitWeixinMessage(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) <= limit {
		return []string{text}
	}
	runes := []rune(text)
	var parts []string
	for len(runes) > 0 {
		end := limit
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			window := string(runes[:end])
			for _, marker := range []string{"\n\n", "\n", "。", "."} {
				if idx := strings.LastIndex(window, marker); idx > 0 {
					end = utf8.RuneCountInString(window[:idx+len(marker)])
					break
				}
			}
		}
		parts = append(parts, string(runes[:end]))
		runes = runes[end:]
	}
	return parts
}
