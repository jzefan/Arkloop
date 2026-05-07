package weixinclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPClient 抽象 HTTP 客户端，便于测试注入。
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client 微信 iLink API 客户端。
type Client struct {
	baseURL    string
	token      string
	httpClient HTTPClient
}

// NewClient 创建微信 iLink 客户端。
func NewClient(baseURL string, token string, httpClient HTTPClient) *Client {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		baseURL:    base,
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}
}

// GetBotQRCode 获取登录二维码（无需鉴权）。
func (c *Client) GetBotQRCode(ctx context.Context) (*QRCodeResp, error) {
	url := c.baseURL + "/ilink/bot/get_bot_qrcode?bot_type=3"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weixin qrcode request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest[QRCodeResp](c.httpClient, req)
}

// GetQRCodeStatus 轮询扫码状态（无需鉴权）。
func (c *Client) GetQRCodeStatus(ctx context.Context, qrcode string) (*QRCodeStatusResp, error) {
	url := c.baseURL + "/ilink/bot/get_qrcode_status?qrcode=" + qrcode
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("weixin qrcode status request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest[QRCodeStatusResp](c.httpClient, req)
}

// SendMessage 发送消息（需要 bot_token 鉴权）。
func (c *Client) SendMessage(ctx context.Context, reqBody *SendMessageRequest) (*SendMessageResponse, error) {
	return callJSON[SendMessageResponse](c, ctx, "POST", "/ilink/bot/sendmessage", map[string]any{
		"msg":       reqBody,
		"base_info": weixinBaseInfo(),
	})
}

// GetUpdates 长轮询收消息（需要 bot_token 鉴权）。
func (c *Client) GetUpdates(ctx context.Context, cursor string) (*GetUpdatesResponse, error) {
	body := map[string]any{
		"get_updates_buf": cursor,
		"base_info":       weixinBaseInfo(),
	}
	return callJSON[GetUpdatesResponse](c, ctx, "POST", "/ilink/bot/getupdates", body)
}

func weixinBaseInfo() map[string]string {
	return map[string]string{"channel_version": "1.0.2"}
}

// callJSON 通用 POST 请求，携带 iLink 鉴权头。
func callJSON[T any](c *Client, ctx context.Context, method, path string, body any) (*T, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("weixin marshal: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("weixin request: %w", err)
	}
	c.setAuthHeaders(req)
	return doRequest[T](c.httpClient, req)
}

// setAuthHeaders 设置 iLink 鉴权头和 X-WECHAT-UIN。
func (c *Client) setAuthHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("X-WECHAT-UIN", randomWeixinUIN())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// randomWeixinUIN 生成随机 X-WECHAT-UIN 值。
func randomWeixinUIN() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	n := binary.BigEndian.Uint32(b[:])
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(uint64(n), 10)))
}

// doRequest 执行 HTTP 请求并解析 iLink envelope 响应。
func doRequest[T any](httpClient HTTPClient, req *http.Request) (*T, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("weixin call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("weixin read: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBody)
		if len(preview) > 500 {
			preview = preview[:500]
		}
		return nil, fmt.Errorf("weixin http %d: %s", resp.StatusCode, preview)
	}

	var envelope struct {
		Ret     int             `json:"ret"`
		Message string          `json:"message,omitempty"`
		Data    json.RawMessage `json:"-"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("weixin decode: %w (body: %s)", err, string(respBody))
	}
	if envelope.Ret != 0 {
		return nil, fmt.Errorf("weixin api error: ret=%d msg=%s", envelope.Ret, envelope.Message)
	}

	var result T
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("weixin decode body: %w", err)
	}
	return &result, nil
}
