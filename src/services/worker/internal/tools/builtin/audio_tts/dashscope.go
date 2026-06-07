package audiotts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arkloop/services/worker/internal/llm"
)

// DashScope multimodal TTS protocol — used by qwen3-tts-* family and the
// rebranded CosyVoice models.
//
// Endpoint: POST {base}/services/aigc/multimodal-generation/generation
// Body:     {"model":"qwen3-tts-flash","input":{"text":"...","voice":"Cherry"},
//            "parameters":{"audio_format":"mp3"}}
// Response: {"output":{"audio":{"url":"...","expires_at":...}}, "usage":{...}}
//
// The URL is an OSS pre-signed link to a wav (default) or mp3 file. We fetch
// it once and surface the bytes back through the same GeneratedSpeech path the
// OpenAI-compatible /audio/speech branch already uses.
//
// CosyVoice "v3.5" does not exist on DashScope as of 2026-05; the closest
// stable replacement is qwen3-tts-flash (production, no date suffix). The
// detection here keys on base_url so any DashScope-hosted TTS model just works.

const (
	dashscopeMultimodalPath    = "/services/aigc/multimodal-generation/generation"
	dashscopeAudioFetchTimeout = 30 * time.Second
)

func isDashScopeConfig(cfg llm.ResolvedGatewayConfig) bool {
	base := strings.ToLower(strings.TrimSpace(cfg.Transport.BaseURL))
	if base == "" {
		return false
	}
	return strings.Contains(base, "dashscope.aliyuncs.com")
}

// dashscopeAPIBase normalises the base URL so we always hit /api/v1, even when
// the credential was configured with the OpenAI-compatible /compatible-mode/v1
// suffix (qwen3-tts does NOT live under compatible-mode).
func dashscopeAPIBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return "https://dashscope.aliyuncs.com/api/v1"
	}
	if strings.HasSuffix(base, "/compatible-mode/v1") {
		base = strings.TrimSuffix(base, "/compatible-mode/v1") + "/api/v1"
	} else if !strings.Contains(base, "/api/v") {
		base = base + "/api/v1"
	}
	return base
}

type dashscopeTTSRequest struct {
	Model      string            `json:"model"`
	Input      dashscopeTTSInput `json:"input"`
	Parameters map[string]any    `json:"parameters,omitempty"`
}

type dashscopeTTSInput struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

type dashscopeTTSResponse struct {
	Output struct {
		Audio struct {
			URL       string `json:"url"`
			ID        string `json:"id"`
			ExpiresAt int64  `json:"expires_at"`
			Data      string `json:"data,omitempty"`
		} `json:"audio"`
		FinishReason string `json:"finish_reason"`
	} `json:"output"`
	Usage struct {
		Characters int `json:"characters"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
}

func synthesizeDashScopeSpeech(ctx context.Context, resolved llm.ResolvedGatewayConfig, req SpeechRequest) (GeneratedSpeech, error) {
	apiKey := strings.TrimSpace(resolved.Transport.APIKey)
	if apiKey == "" {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS: API key is empty")
	}
	base := dashscopeAPIBase(resolved.Transport.BaseURL)
	model := strings.TrimSpace(resolved.Model)
	if model == "" {
		model = "qwen3-tts-flash"
	}
	voice := strings.TrimSpace(req.Voice)
	if voice == "" {
		voice = "Cherry"
	}

	parameters := map[string]any{}
	if req.ResponseFormat != "" {
		parameters["audio_format"] = req.ResponseFormat
	}

	body, err := json.Marshal(dashscopeTTSRequest{
		Model:      model,
		Input:      dashscopeTTSInput{Text: req.Text, Voice: voice},
		Parameters: parameters,
	})
	if err != nil {
		return GeneratedSpeech{}, err
	}

	httpCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, base+dashscopeMultimodalPath, bytes.NewReader(body))
	if err != nil {
		return GeneratedSpeech{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range resolved.Transport.DefaultHeaders {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			httpReq.Header.Set(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS network error: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return GeneratedSpeech{}, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 600))
	}

	var parsed dashscopeTTSResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS response not JSON: %s", truncate(string(respBody), 400))
	}
	if parsed.Code != "" {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS error %s: %s", parsed.Code, parsed.Message)
	}
	url := strings.TrimSpace(parsed.Output.Audio.URL)
	if url == "" {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS response missing audio.url: %s", truncate(string(respBody), 400))
	}

	fetchCtx, fetchCancel := context.WithTimeout(ctx, dashscopeAudioFetchTimeout)
	defer fetchCancel()
	fetchReq, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		return GeneratedSpeech{}, err
	}
	fetchResp, err := http.DefaultClient.Do(fetchReq)
	if err != nil {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS audio download network error: %w", err)
	}
	defer fetchResp.Body.Close()
	audioBytes, readErr := io.ReadAll(io.LimitReader(fetchResp.Body, maxSpeechBytes+1))
	if readErr != nil {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS audio download read: %w", readErr)
	}
	if fetchResp.StatusCode < 200 || fetchResp.StatusCode >= 300 {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS audio download HTTP %d: %s", fetchResp.StatusCode, truncate(string(audioBytes), 200))
	}
	if len(audioBytes) > maxSpeechBytes {
		return GeneratedSpeech{}, fmt.Errorf("DashScope TTS audio exceeds %d bytes", maxSpeechBytes)
	}

	mime := strings.ToLower(strings.TrimSpace(strings.SplitN(fetchResp.Header.Get("Content-Type"), ";", 2)[0]))
	if mime == "" || mime == "application/octet-stream" || mime == "binary/octet-stream" {
		switch {
		case strings.Contains(url, ".mp3"):
			mime = "audio/mpeg"
		case strings.Contains(url, ".wav"):
			mime = "audio/wav"
		case strings.Contains(url, ".aac"):
			mime = "audio/aac"
		case strings.Contains(url, ".flac"):
			mime = "audio/flac"
		case strings.Contains(url, ".opus"):
			mime = "audio/opus"
		default:
			mime = "audio/wav"
		}
	}

	return GeneratedSpeech{
		Bytes:        audioBytes,
		MimeType:     mime,
		ProviderKind: "dashscope-qwen3-tts",
		Model:        model,
	}, nil
}
