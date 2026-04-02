package onebotclient

import "encoding/json"

// OneBot11 消息段
type MessageSegment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// 文本消息段 data
type TextData struct {
	Text string `json:"text"`
}

// 事件基础字段
type Event struct {
	Time        int64           `json:"time"`
	SelfID      json.Number     `json:"self_id"`
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type,omitempty"`
	SubType     string          `json:"sub_type,omitempty"`
	MessageID   json.Number     `json:"message_id,omitempty"`
	UserID      json.Number     `json:"user_id,omitempty"`
	GroupID     json.Number     `json:"group_id,omitempty"`
	RawMessage  string          `json:"raw_message,omitempty"`
	Message     json.RawMessage `json:"message,omitempty"`
	Sender      *Sender         `json:"sender,omitempty"`
	NoticeType  string          `json:"notice_type,omitempty"`
	RequestType string          `json:"request_type,omitempty"`
	MetaEvent   string          `json:"meta_event_type,omitempty"`

	// 群通知字段
	OperatorID json.Number `json:"operator_id,omitempty"`
	Comment    string      `json:"comment,omitempty"`
	Flag       string      `json:"flag,omitempty"`
}

type Sender struct {
	UserID   json.Number `json:"user_id,omitempty"`
	Nickname string      `json:"nickname,omitempty"`
	Card     string      `json:"card,omitempty"`
	Role     string      `json:"role,omitempty"`
}

// IsMessageEvent 判断是否为消息事件
func (e *Event) IsMessageEvent() bool {
	return e.PostType == "message"
}

// IsPrivateMessage 私聊消息
func (e *Event) IsPrivateMessage() bool {
	return e.PostType == "message" && e.MessageType == "private"
}

// IsGroupMessage 群聊消息
func (e *Event) IsGroupMessage() bool {
	return e.PostType == "message" && e.MessageType == "group"
}

// IsHeartbeat 心跳事件
func (e *Event) IsHeartbeat() bool {
	return e.PostType == "meta_event" && e.MetaEvent == "heartbeat"
}

// IsLifecycle 生命周期事件
func (e *Event) IsLifecycle() bool {
	return e.PostType == "meta_event" && e.MetaEvent == "lifecycle"
}

// PlainText 从 message 字段提取纯文本
func (e *Event) PlainText() string {
	if e.RawMessage != "" {
		return e.RawMessage
	}
	// 尝试解析 message array
	var segments []MessageSegment
	if err := json.Unmarshal(e.Message, &segments); err != nil {
		return ""
	}
	var buf []byte
	for _, seg := range segments {
		if seg.Type != "text" {
			continue
		}
		var td TextData
		if err := json.Unmarshal(seg.Data, &td); err != nil {
			continue
		}
		buf = append(buf, td.Text...)
	}
	return string(buf)
}

// SenderDisplayName 返回发送者展示名（优先群名片）
func (e *Event) SenderDisplayName() string {
	if e.Sender == nil {
		return ""
	}
	if e.Sender.Card != "" {
		return e.Sender.Card
	}
	return e.Sender.Nickname
}

// --- send_msg 请求/响应 ---

type SendMsgRequest struct {
	MessageType string           `json:"message_type"`
	UserID      string           `json:"user_id,omitempty"`
	GroupID     string           `json:"group_id,omitempty"`
	Message     []MessageSegment `json:"message"`
}

type SendMsgResponse struct {
	MessageID json.Number `json:"message_id"`
}

// --- get_login_info 响应 ---

type LoginInfo struct {
	UserID   json.Number `json:"user_id"`
	Nickname string      `json:"nickname"`
}

// TextSegments 将纯文本构造为消息段数组
func TextSegments(text string) []MessageSegment {
	data, _ := json.Marshal(TextData{Text: text})
	return []MessageSegment{{Type: "text", Data: data}}
}
