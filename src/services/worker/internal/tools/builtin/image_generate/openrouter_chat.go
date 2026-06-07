package imagegenerate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arkloop/services/worker/internal/llm"
)

// openRouterBaseURLMarker 用来识别一个 resolved gateway config 是否指向 OpenRouter。
// OpenRouter 的图像模型（openai/gpt-5-image-mini、google/gemini-*-image 等）不走
// OpenAI 原生的 /v1/images/generations 端点 —— 那个端点 OpenRouter 不实现，请求会
// 直接落到 OpenRouter 的 marketing 页面返回 HTML 404。
// 实际可用路径是 /v1/chat/completions + modalities=["image","text"]，模型在
// response.choices[0].message.images[] 里返回 base64 data URI。
const openRouterBaseURLMarker = "openrouter.ai"

// isOpenRouterConfig 判断 resolved gateway config 是否是 OpenRouter 端点。
// 只匹配 base_url 中含 "openrouter.ai"，不依赖凭据名（凭据名是部署侧可改的）。
func isOpenRouterConfig(cfg llm.ResolvedGatewayConfig) bool {
	base := strings.ToLower(strings.TrimSpace(cfg.Transport.BaseURL))
	if base == "" {
		return false
	}
	return strings.Contains(base, openRouterBaseURLMarker)
}

type openRouterChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openRouterChatRequest struct {
	Model      string                  `json:"model"`
	Messages   []openRouterChatMessage `json:"messages"`
	Modalities []string                `json:"modalities"`
}

type openRouterImageBlock struct {
	Type     string `json:"type"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

type openRouterChatResponse struct {
	Error   *openRouterChatError    `json:"error,omitempty"`
	Choices []openRouterChatChoice  `json:"choices"`
	ID      string                  `json:"id"`
	Model   string                  `json:"model"`
}

type openRouterChatChoice struct {
	Message struct {
		Role    string                 `json:"role"`
		Content any                    `json:"content"`
		Refusal *string                `json:"refusal,omitempty"`
		Images  []openRouterImageBlock `json:"images,omitempty"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type openRouterChatError struct {
	Message string `json:"message"`
	Code    any    `json:"code"`
	Type    string `json:"type"`
}

// generateImageViaOpenRouterChat 实现 OpenRouter chat-completions 出图路径。
//
// 输入：resolved.Transport（APIKey + BaseURL）+ resolved.Model（如 "openai/gpt-5-image-mini"）+
// req.Prompt + req.InputImages（每个 ContentPart 内嵌图片 bytes + mime_type）。
//
// 行为：
//   1) 构造一条 user message，content 由 prompt + 每张 input image（image_url 块，
//      url 为 data:<mime>;base64,<...>）组成。
//   2) POST {base_url}/chat/completions，modalities=["image","text"]。
//   3) 解析 response.choices[0].message.images[0].image_url.url，要求是 data: URI，
//      抽出 base64 与 mime_type，解码成 bytes 返回。
//
// 错误处理：将 OpenRouter 的 JSON 错误体原样写入 GatewayError.Details.provider_error_body，
// HTTP status 写入 Details.status_code，方便上层暴露给用户。
func generateImageViaOpenRouterChat(ctx context.Context, resolved llm.ResolvedGatewayConfig, req llm.ImageGenerationRequest) (llm.GeneratedImage, error) {
	base := strings.TrimRight(strings.TrimSpace(resolved.Transport.BaseURL), "/")
	if base == "" {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassConfigMissing,
			Message:    "OpenRouter base_url is empty",
		}
	}
	if strings.TrimSpace(resolved.Transport.APIKey) == "" {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassConfigMissing,
			Message:    "OpenRouter API key is empty",
		}
	}

	// 构造 message content：prompt 文本 + 每张 input 图片
	parts := []map[string]any{
		{"type": "text", "text": req.Prompt},
	}
	for _, img := range req.InputImages {
		if img.Attachment == nil || len(img.Data) == 0 {
			continue
		}
		mime := strings.TrimSpace(img.Attachment.MimeType)
		if mime == "" {
			mime = "image/png"
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(img.Data))
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": dataURL},
		})
	}

	payload := openRouterChatRequest{
		Model:      resolved.Model,
		Messages:   []openRouterChatMessage{{Role: "user", Content: parts}},
		Modalities: []string{"image", "text"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassInternalError,
			Message:    fmt.Sprintf("OpenRouter request marshal failed: %s", err.Error()),
		}
	}

	endpoint := base + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassInternalError,
			Message:    fmt.Sprintf("OpenRouter request build failed: %s", err.Error()),
		}
	}
	httpReq.Header.Set("Authorization", "Bearer "+resolved.Transport.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassProviderRetryable,
			Message:    fmt.Sprintf("OpenRouter network error: %s", err.Error()),
			Details:    map[string]any{"provider_kind": "openrouter", "network_attempted": true},
		}
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		class := llm.ErrorClassProviderNonRetryable
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			class = llm.ErrorClassProviderRetryable
		}
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: class,
			Message:    fmt.Sprintf("OpenRouter image request failed (status %d)", resp.StatusCode),
			Details: map[string]any{
				"provider_kind":       "openrouter",
				"status_code":         resp.StatusCode,
				"provider_error_body": truncateBody(respBytes, 2048),
				"network_attempted":   true,
			},
		}
	}

	var parsed openRouterChatResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassProviderNonRetryable,
			Message:    fmt.Sprintf("OpenRouter response not JSON: %s", err.Error()),
			Details:    map[string]any{"provider_error_body": truncateBody(respBytes, 2048)},
		}
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassProviderNonRetryable,
			Message:    "OpenRouter error: " + parsed.Error.Message,
			Details: map[string]any{
				"provider_kind":       "openrouter",
				"provider_error_body": truncateBody(respBytes, 2048),
			},
		}
	}
	if len(parsed.Choices) == 0 || len(parsed.Choices[0].Message.Images) == 0 {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassProviderNonRetryable,
			Message:    "OpenRouter response has no images",
			Details: map[string]any{
				"provider_kind":       "openrouter",
				"provider_error_body": truncateBody(respBytes, 2048),
			},
		}
	}

	url := parsed.Choices[0].Message.Images[0].ImageURL.URL
	mime, data, err := decodeDataURL(url)
	if err != nil {
		return llm.GeneratedImage{}, llm.GatewayError{
			ErrorClass: llm.ErrorClassProviderNonRetryable,
			Message:    fmt.Sprintf("OpenRouter image url decode failed: %s", err.Error()),
		}
	}

	return llm.GeneratedImage{
		Bytes:        data,
		MimeType:     mime,
		ProviderKind: "openrouter",
		Model:        resolved.Model,
	}, nil
}

// decodeDataURL 解析 "data:<mime>;base64,<payload>" 形式的 data URI。
func decodeDataURL(url string) (string, []byte, error) {
	if !strings.HasPrefix(url, "data:") {
		return "", nil, fmt.Errorf("image url is not a data URI")
	}
	rest := url[len("data:"):]
	commaIdx := strings.Index(rest, ",")
	if commaIdx < 0 {
		return "", nil, fmt.Errorf("data URI missing comma")
	}
	header := rest[:commaIdx]
	payload := rest[commaIdx+1:]
	mime := "image/png"
	isBase64 := false
	for _, segment := range strings.Split(header, ";") {
		seg := strings.TrimSpace(segment)
		if seg == "base64" {
			isBase64 = true
			continue
		}
		if strings.HasPrefix(seg, "image/") {
			mime = seg
		}
	}
	if !isBase64 {
		return "", nil, fmt.Errorf("data URI must be base64-encoded")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, fmt.Errorf("base64 decode: %w", err)
	}
	return mime, data, nil
}

func truncateBody(raw []byte, max int) string {
	if len(raw) <= max {
		return string(raw)
	}
	return string(raw[:max]) + "...(truncated)"
}
