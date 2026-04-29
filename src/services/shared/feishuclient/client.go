package feishuclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultFeishuBaseURL = "https://open.feishu.cn"
	defaultLarkBaseURL   = "https://open.larksuite.com"
	tokenRefreshSkew     = 60 * time.Second
	maxBodyPreviewBytes  = 4096
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	AppID     string
	AppSecret string
	Domain    string
	baseURL   string
}

type Client struct {
	appID     string
	appSecret string
	baseURL   string

	httpClient HTTPClient
	now        func() time.Time

	mu          sync.Mutex
	tenantToken string
	tokenExpiry time.Time
}

func NewClient(cfg Config, httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		appID:      strings.TrimSpace(cfg.AppID),
		appSecret:  strings.TrimSpace(cfg.AppSecret),
		baseURL:    configBaseURL(cfg),
		httpClient: httpClient,
		now:        time.Now,
	}
}

func BaseURL(domain string) string {
	value := strings.TrimSpace(domain)
	switch strings.ToLower(value) {
	case "", "feishu", defaultFeishuBaseURL, "open.feishu.cn":
		return defaultFeishuBaseURL
	case "lark", defaultLarkBaseURL, "open.larksuite.com":
		return defaultLarkBaseURL
	default:
		return defaultFeishuBaseURL
	}
}

func configBaseURL(cfg Config) string {
	if override := strings.TrimSpace(cfg.baseURL); override != "" {
		return strings.TrimRight(override, "/")
	}
	return BaseURL(cfg.Domain)
}

type BotInfo struct {
	AppName     string   `json:"app_name"`
	AvatarURL   string   `json:"avatar_url"`
	IPWhiteList []string `json:"ip_white_list"`
	OpenID      string   `json:"open_id"`
	UserID      string   `json:"user_id"`
}

type SentMessage struct {
	MessageID  string `json:"message_id"`
	RootID     string `json:"root_id"`
	ParentID   string `json:"parent_id"`
	ThreadID   string `json:"thread_id"`
	ChatID     string `json:"chat_id"`
	MsgType    string `json:"msg_type"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
}

func (c *Client) GetBotInfo(ctx context.Context) (*BotInfo, error) {
	var out struct {
		Bot BotInfo `json:"bot"`
	}
	if err := c.call(ctx, http.MethodGet, "/open-apis/bot/v3/info", "", nil, &out); err != nil {
		return nil, err
	}
	return &out.Bot, nil
}

func (c *Client) SendText(ctx context.Context, receiveIDType, receiveID, text, uuid string) (*SentMessage, error) {
	receiveIDType = strings.TrimSpace(receiveIDType)
	receiveID = strings.TrimSpace(receiveID)
	text = strings.TrimSpace(text)
	if receiveID == "" {
		return nil, fmt.Errorf("feishuclient: receive_id must not be empty")
	}
	if text == "" {
		return nil, fmt.Errorf("feishuclient: text must not be empty")
	}

	q := url.Values{}
	q.Set("receive_id_type", receiveIDType)
	req := messageRequest{
		ReceiveID: receiveID,
		MsgType:   "text",
		Content:   textContent(text),
		UUID:      strings.TrimSpace(uuid),
	}

	var out SentMessage
	if err := c.call(ctx, http.MethodPost, "/open-apis/im/v1/messages", q.Encode(), req, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.MessageID) == "" {
		return nil, fmt.Errorf("feishuclient: message_id is empty")
	}
	return &out, nil
}

func (c *Client) ReplyText(ctx context.Context, messageID, text string, replyInThread bool, uuid string) (*SentMessage, error) {
	messageID = strings.TrimSpace(messageID)
	text = strings.TrimSpace(text)
	if messageID == "" {
		return nil, fmt.Errorf("feishuclient: message_id must not be empty")
	}
	if text == "" {
		return nil, fmt.Errorf("feishuclient: text must not be empty")
	}

	req := messageRequest{
		MsgType:       "text",
		Content:       textContent(text),
		ReplyInThread: replyInThread,
		UUID:          strings.TrimSpace(uuid),
	}
	path := "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/reply"
	var out SentMessage
	if err := c.call(ctx, http.MethodPost, path, "", req, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.MessageID) == "" {
		return nil, fmt.Errorf("feishuclient: message_id is empty")
	}
	return &out, nil
}

type messageRequest struct {
	ReceiveID     string `json:"receive_id,omitempty"`
	MsgType       string `json:"msg_type"`
	Content       string `json:"content"`
	UUID          string `json:"uuid,omitempty"`
	ReplyInThread bool   `json:"reply_in_thread,omitempty"`
}

func textContent(text string) string {
	raw, _ := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	return string(raw)
}

func (c *Client) call(ctx context.Context, method, path, rawQuery string, body any, out any) error {
	if err := c.validateCredentials(); err != nil {
		return err
	}
	token, err := c.tenantAccessToken(ctx)
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("feishuclient: marshal request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	endpoint := c.baseURL + path
	if rawQuery != "" {
		endpoint += "?" + rawQuery
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("feishuclient: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doEnvelope(req, out)
}

func (c *Client) validateCredentials() error {
	if strings.TrimSpace(c.appID) == "" {
		return fmt.Errorf("feishuclient: app_id must not be empty")
	}
	if strings.TrimSpace(c.appSecret) == "" {
		return fmt.Errorf("feishuclient: app_secret must not be empty")
	}
	return nil
}

func (c *Client) tenantAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tenantToken != "" && c.now().Add(tokenRefreshSkew).Before(c.tokenExpiry) {
		return c.tenantToken, nil
	}

	token, expiry, err := c.fetchTenantAccessToken(ctx)
	if err != nil {
		return "", err
	}

	c.tenantToken = token
	c.tokenExpiry = expiry
	return token, nil
}

func (c *Client) fetchTenantAccessToken(ctx context.Context) (string, time.Time, error) {
	payload, err := json.Marshal(map[string]string{
		"app_id":     c.appID,
		"app_secret": c.appSecret,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("feishuclient: marshal token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(payload))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("feishuclient: new token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var out struct {
		TenantAccessToken string `json:"tenant_access_token"`
		ExpireSeconds     int64  `json:"expire"`
	}
	if err := c.doEnvelope(req, &out); err != nil {
		return "", time.Time{}, err
	}
	if strings.TrimSpace(out.TenantAccessToken) == "" {
		return "", time.Time{}, fmt.Errorf("feishuclient: tenant_access_token is empty")
	}
	return out.TenantAccessToken, c.now().Add(time.Duration(out.ExpireSeconds) * time.Second), nil
}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) doEnvelope(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("feishuclient: do %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyPreviewBytes+1))
	if err != nil {
		return fmt.Errorf("feishuclient: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishuclient: status %d: %s", resp.StatusCode, preview(body))
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("feishuclient: decode response: %w", err)
	}
	if env.Code != 0 {
		return fmt.Errorf("feishuclient: code=%d msg=%s", env.Code, env.Msg)
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("feishuclient: decode data: %w", err)
		}
		return nil
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("feishuclient: decode body: %w", err)
		}
	}
	return nil
}

func preview(body []byte) string {
	if len(body) > maxBodyPreviewBytes {
		body = body[:maxBodyPreviewBytes]
	}
	return string(body)
}
