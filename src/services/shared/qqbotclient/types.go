package qqbotclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	EventReady                = "READY"
	EventC2CMessageCreate     = "C2C_MESSAGE_CREATE"
	EventGroupAtMessageCreate = "GROUP_AT_MESSAGE_CREATE"

	ScopeC2C   = "c2c"
	ScopeGroup = "group"

	IntentDirectMessages      = 1 << 12
	IntentC2CGroupAtMessages  = 1 << 25
	IntentPublicGuildMessages = 1 << 30
	DefaultIntents            = IntentC2CGroupAtMessages
)

type Credentials struct {
	AppID        string
	ClientSecret string
}

func ParseCredentials(configJSON json.RawMessage, token string) (Credentials, error) {
	cfg := struct {
		AppID  string `json:"app_id"`
		AppID2 string `json:"appId"`
	}{}
	if len(configJSON) > 0 {
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return Credentials{}, fmt.Errorf("qqbot config_json must be a valid JSON object")
		}
	}
	appID := strings.TrimSpace(firstNonEmpty(cfg.AppID, cfg.AppID2))
	secret := strings.TrimSpace(token)
	if left, right, ok := strings.Cut(secret, ":"); ok {
		if appID == "" {
			appID = strings.TrimSpace(left)
		}
		secret = strings.TrimSpace(right)
	}
	if appID == "" {
		return Credentials{}, fmt.Errorf("qqbot app_id is required")
	}
	if secret == "" {
		return Credentials{}, fmt.Errorf("qqbot client_secret is required")
	}
	return Credentials{AppID: appID, ClientSecret: secret}, nil
}

type AccessToken struct {
	Token     string
	ExpiresIn int64
}

type GatewayEvent struct {
	Op       int
	Type     string
	Sequence int64
	Data     json.RawMessage
}

type GatewayEventHandler func(ctx context.Context, event GatewayEvent)

type ReadyEvent struct {
	Version   int    `json:"version,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	User      User   `json:"user,omitempty"`
}

type MessageCreateEvent struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	Timestamp   string    `json:"timestamp,omitempty"`
	Author      User      `json:"author"`
	Member      Member    `json:"member,omitempty"`
	GroupOpenID string    `json:"group_openid,omitempty"`
	GroupID     string    `json:"group_id,omitempty"`
	OpenID      string    `json:"openid,omitempty"`
	UserOpenID  string    `json:"user_openid,omitempty"`
	Mentions    []Mention `json:"mentions,omitempty"`
}

type User struct {
	ID           string `json:"id,omitempty"`
	OpenID       string `json:"openid,omitempty"`
	UserOpenID   string `json:"user_openid,omitempty"`
	MemberOpenID string `json:"member_openid,omitempty"`
	Username     string `json:"username,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
}

type Member struct {
	User     User   `json:"user,omitempty"`
	Nick     string `json:"nick,omitempty"`
	Nickname string `json:"nickname,omitempty"`
}

type Mention struct {
	ID           string `json:"id,omitempty"`
	UserOpenID   string `json:"user_openid,omitempty"`
	MemberOpenID string `json:"member_openid,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
	Username     string `json:"username,omitempty"`
	IsYou        bool   `json:"is_you,omitempty"`
}

func (m MessageCreateEvent) SenderOpenID() string {
	return strings.TrimSpace(firstNonEmpty(
		m.Author.UserOpenID,
		m.Author.MemberOpenID,
		m.Author.OpenID,
		m.Author.ID,
		m.Member.User.UserOpenID,
		m.Member.User.MemberOpenID,
		m.Member.User.OpenID,
		m.Member.User.ID,
		m.UserOpenID,
		m.OpenID,
	))
}

func (m MessageCreateEvent) SenderDisplayName() string {
	return strings.TrimSpace(firstNonEmpty(
		m.Member.Nick,
		m.Member.Nickname,
		m.Author.Nickname,
		m.Author.Username,
		m.SenderOpenID(),
	))
}

func (m MessageCreateEvent) ContentWithoutSelfMentions() string {
	content := m.Content
	for _, mention := range m.Mentions {
		if !mention.IsYou {
			continue
		}
		openid := strings.TrimSpace(firstNonEmpty(mention.MemberOpenID, mention.ID, mention.UserOpenID))
		if openid == "" {
			continue
		}
		content = strings.ReplaceAll(content, "<@"+openid+">", "")
		content = strings.ReplaceAll(content, "<@!"+openid+">", "")
	}
	return strings.TrimSpace(content)
}

func (m MessageCreateEvent) GroupTarget() string {
	return strings.TrimSpace(firstNonEmpty(m.GroupOpenID, m.GroupID))
}

type SendMessageResponse struct {
	ID        string          `json:"id,omitempty"`
	MessageID string          `json:"message_id,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

func (r SendMessageResponse) PlatformMessageID() string {
	return strings.TrimSpace(firstNonEmpty(r.ID, r.MessageID))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
