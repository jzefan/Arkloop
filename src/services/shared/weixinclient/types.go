package weixinclient

// QRCodeResp 获取登录二维码响应。
type QRCodeResp struct {
	Ret              int    `json:"ret"`
	Qrcode           string `json:"qrcode"`
	QrcodeImgContent string `json:"qrcode_img_content"`
}

// QRCodeStatusResp 轮询扫码状态响应。
type QRCodeStatusResp struct {
	Ret      int    `json:"ret"`
	Status   string `json:"status"`
	BotToken string `json:"bot_token,omitempty"`
	BaseURL  string `json:"baseurl,omitempty"`
}

// SendMessageRequest iLink 发送消息请求体。
type SendMessageRequest struct {
	ToUserID     string        `json:"to_user_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ContextToken string        `json:"context_token,omitempty"`
	ItemList     []MessageItem `json:"item_list"`
}

// MessageItem 消息片段。
type MessageItem struct {
	Type     int       `json:"type"`
	TextItem *TextItem `json:"text_item,omitempty"`
}

// TextItem 文本内容。
type TextItem struct {
	Text string `json:"text"`
}

// SendMessageResponse 发送消息响应。
type SendMessageResponse struct {
	Ret       int    `json:"ret"`
	MessageID string `json:"message_id,omitempty"`
}

// GetUpdatesResponse 长轮询收消息响应。
type GetUpdatesResponse struct {
	Ret                  int             `json:"ret"`
	Msgs                 []WeixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf        string          `json:"get_updates_buf"`
	LongpollingTimeoutMs int             `json:"longpolling_timeout_ms"`
}

// WeixinMessage 微信消息。
type WeixinMessage struct {
	FromUserID   string        `json:"from_user_id"`
	ToUserID     string        `json:"to_user_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ContextToken string        `json:"context_token"`
	ItemList     []MessageItem `json:"item_list"`
	GroupID      string        `json:"group_id,omitempty"`
}
