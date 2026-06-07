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

	sharedconfig "arkloop/services/shared/config"
	"arkloop/services/shared/objectstore"
	workerdata "arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

const (
	audioTTSConfigKey     = "audio_tts.model"
	defaultArtifactName   = "voice_over"
	defaultVoice          = "alloy"
	defaultResponseFormat = "mp3"
	httpTimeout           = 90 * time.Second
	maxSpeechBytes        = 50 << 20
)

type ToolExecutor struct {
	store         objectstore.Store
	db            workerdata.QueryDB
	config        sharedconfig.Resolver
	routingLoader *routing.ConfigLoader
	synthesize    func(context.Context, llm.ResolvedGatewayConfig, SpeechRequest) (GeneratedSpeech, error)
}

func NewToolExecutor(
	store objectstore.Store,
	db workerdata.QueryDB,
	config sharedconfig.Resolver,
	routingLoader *routing.ConfigLoader,
) *ToolExecutor {
	return &ToolExecutor{store: store, db: db, config: config, routingLoader: routingLoader}
}

type SpeechRequest struct {
	Text           string
	Voice          string
	ResponseFormat string
	Speed          float64
}

type GeneratedSpeech struct {
	Bytes        []byte
	MimeType     string
	ProviderKind string
	Model        string
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if e == nil || e.store == nil {
		return errResult("tool.not_configured", "audio TTS storage is not configured", started)
	}
	if execCtx.AccountID == nil {
		return errResult("tool.execution_failed", "account context is required", started)
	}
	text := strings.TrimSpace(stringArg(args, "text"))
	if text == "" {
		return errResult("tool.args_invalid", "parameter text is required", started)
	}
	voice := strings.TrimSpace(stringArg(args, "voice"))
	if voice == "" {
		voice = defaultVoice
	}
	format := strings.ToLower(strings.TrimSpace(stringArg(args, "response_format")))
	if format == "" {
		format = defaultResponseFormat
	}
	if !validResponseFormat(format) {
		return errResult("tool.args_invalid", "response_format must be mp3, wav, opus, aac, flac, or pcm", started)
	}
	speed := numberArg(args, "speed", 1.0)
	if speed < 0.25 {
		speed = 0.25
	}
	if speed > 4.0 {
		speed = 4.0
	}

	var resolved llm.ResolvedGatewayConfig
	var err error
	if e.synthesize == nil {
		selected, selectErr := e.resolveSelectedRoute(ctx, *execCtx.AccountID, strings.TrimSpace(stringArg(args, "model_selector")))
		if selectErr != nil {
			return errResult("tool.not_configured", selectErr.Error(), started)
		}
		resolved, err = pipeline.ResolveGatewayConfigFromSelectedRouteForRequest(ctx, *selected, false, 0)
		if err != nil {
			return errResult("tool.execution_failed", "resolve TTS model failed: "+err.Error(), started)
		}
	}

	request := SpeechRequest{Text: text, Voice: voice, ResponseFormat: format, Speed: speed}
	synthesizer := e.synthesize
	if synthesizer == nil {
		// 按 base_url 分流：DashScope (qwen3-tts / CosyVoice 系列) 走原生 multimodal-generation
		// 端点，其它 (OpenAI / OpenRouter 等) 走 /audio/speech 兼容协议。
		if isDashScopeConfig(resolved) {
			synthesizer = synthesizeDashScopeSpeech
		} else {
			synthesizer = synthesizeOpenAISpeech
		}
	}
	speech, err := synthesizer(ctx, resolved, request)
	if err != nil {
		return errResult("provider.non_retryable", err.Error(), started)
	}
	if len(speech.Bytes) == 0 {
		return errResult("tool.execution_failed", "TTS provider returned empty audio bytes", started)
	}

	contentType := normalizeAudioContentType(speech.MimeType, format)
	artifactBase := sanitizeArtifactName(strings.TrimSpace(stringArg(args, "artifact_name")))
	if artifactBase == "" {
		artifactBase = defaultArtifactName
	}
	filename := artifactBase + fileExtForContentType(contentType, format)
	key := buildArtifactKey(execCtx, filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		value := execCtx.ThreadID.String()
		threadID = &value
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), execCtx.AccountID.String(), threadID)
	if err := e.store.PutObject(ctx, key, speech.Bytes, objectstore.PutOptions{
		ContentType: contentType,
		Metadata:    metadata,
	}); err != nil {
		return errResult("tool.upload_failed", "save TTS artifact: "+err.Error(), started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"provider":  speech.ProviderKind,
			"model":     speech.Model,
			"mime_type": contentType,
			"bytes":     len(speech.Bytes),
			"artifacts": []map[string]any{
				{
					"key":       key,
					"filename":  filename,
					"size":      len(speech.Bytes),
					"mime_type": contentType,
					"title":     artifactBase,
					"display":   "inline",
				},
			},
		},
		DurationMs: durationMs(started),
	}
}

func (e *ToolExecutor) IsAvailableForAccount(ctx context.Context, accountID uuid.UUID) bool {
	if accountID == uuid.Nil {
		return false
	}
	if e.synthesize != nil {
		return true
	}
	if _, err := e.resolveSelectedRoute(ctx, accountID, ""); err == nil {
		return true
	}
	if e.routingLoader == nil {
		return false
	}
	cfg, err := e.routingLoader.Load(ctx, &accountID)
	if err != nil || len(cfg.Routes) == 0 {
		return false
	}
	for _, route := range cfg.Routes {
		caps := routing.RouteModelCapabilities(route)
		model := strings.ToLower(strings.TrimSpace(route.Model))
		if caps.SupportsOutputModality("audio") || strings.Contains(model, "tts") || strings.Contains(model, "speech") {
			return true
		}
	}
	return false
}

func (e *ToolExecutor) resolveSelectedRoute(ctx context.Context, accountID uuid.UUID, explicitSelector string) (*routing.SelectedProviderRoute, error) {
	if e.routingLoader == nil {
		return nil, fmt.Errorf("audio TTS routing is not configured")
	}
	selector := strings.TrimSpace(explicitSelector)
	if selector == "" && e.db != nil {
		_ = e.db.QueryRow(ctx,
			`SELECT value FROM account_entitlement_overrides
			  WHERE account_id = $1 AND key = $2
			    AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
			  LIMIT 1`,
			accountID, audioTTSConfigKey,
		).Scan(&selector)
		selector = strings.TrimSpace(selector)
	}
	if selector == "" && e.config != nil {
		if value, err := e.config.Resolve(ctx, audioTTSConfigKey, sharedconfig.Scope{}); err == nil {
			selector = strings.TrimSpace(value)
		}
	}
	if selector == "" {
		return nil, fmt.Errorf("audio TTS model is not configured")
	}

	cfg, err := e.routingLoader.Load(ctx, &accountID)
	if err != nil {
		return nil, fmt.Errorf("load audio TTS routing config failed: %w", err)
	}
	credName, modelName, exact := splitModelSelector(selector)
	if exact {
		if route, cred, ok := cfg.GetHighestPriorityRouteByCredentialAndModel(credName, modelName, map[string]any{}); ok {
			return &routing.SelectedProviderRoute{Route: route, Credential: cred}, nil
		}
		return nil, fmt.Errorf("audio TTS route not found for selector: %s", selector)
	}
	if route, cred, ok := cfg.GetHighestPriorityRouteByModel(selector, map[string]any{}); ok {
		return &routing.SelectedProviderRoute{Route: route, Credential: cred}, nil
	}
	return nil, fmt.Errorf("audio TTS route not found for selector: %s", selector)
}

func synthesizeOpenAISpeech(ctx context.Context, resolved llm.ResolvedGatewayConfig, req SpeechRequest) (GeneratedSpeech, error) {
	base := strings.TrimRight(strings.TrimSpace(resolved.Transport.BaseURL), "/")
	if base == "" {
		return GeneratedSpeech{}, fmt.Errorf("TTS provider base URL is empty")
	}
	if strings.TrimSpace(resolved.Transport.APIKey) == "" {
		return GeneratedSpeech{}, fmt.Errorf("TTS provider API key is empty")
	}
	body := map[string]any{
		"model":           resolved.Model,
		"input":           req.Text,
		"voice":           req.Voice,
		"response_format": req.ResponseFormat,
		"speed":           req.Speed,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return GeneratedSpeech{}, err
	}
	httpCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, base+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return GeneratedSpeech{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+resolved.Transport.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range resolved.Transport.DefaultHeaders {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			httpReq.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return GeneratedSpeech{}, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxSpeechBytes+1)
	data, readErr := io.ReadAll(limited)
	if readErr != nil {
		return GeneratedSpeech{}, readErr
	}
	if len(data) > maxSpeechBytes {
		return GeneratedSpeech{}, fmt.Errorf("TTS provider response exceeds %d bytes", maxSpeechBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GeneratedSpeech{}, fmt.Errorf("TTS provider returned HTTP %d: %s", resp.StatusCode, truncate(string(data), 600))
	}
	return GeneratedSpeech{
		Bytes:        data,
		MimeType:     resp.Header.Get("Content-Type"),
		ProviderKind: "openai-compatible",
		Model:        resolved.Model,
	}, nil
}

func splitModelSelector(selector string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(selector), "^", 2)
	if len(parts) != 2 {
		return "", strings.TrimSpace(selector), false
	}
	credName := strings.TrimSpace(parts[0])
	modelName := strings.TrimSpace(parts[1])
	if credName == "" || modelName == "" {
		return "", strings.TrimSpace(selector), false
	}
	return credName, modelName, true
}

func validResponseFormat(format string) bool {
	switch format {
	case "mp3", "wav", "opus", "aac", "flac", "pcm":
		return true
	default:
		return false
	}
}

func normalizeAudioContentType(contentType, format string) string {
	base := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.HasPrefix(base, "audio/") {
		return base
	}
	switch format {
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "pcm":
		return "audio/L16"
	default:
		return "audio/mpeg"
	}
}

func fileExtForContentType(contentType, format string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg", "audio/opus":
		return ".opus"
	case "audio/aac":
		return ".aac"
	case "audio/flac":
		return ".flac"
	case "audio/l16":
		return ".pcm"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	default:
		return "." + format
	}
}

func stringArg(args map[string]any, key string) string {
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

func numberArg(args map[string]any, key string, fallback float64) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return fallback
}

func sanitizeArtifactName(raw string) string {
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteByte('_')
		}
	}
	cleaned := strings.Trim(b.String(), "-_")
	if len(cleaned) > 80 {
		cleaned = cleaned[:80]
	}
	return cleaned
}

func buildArtifactKey(execCtx tools.ExecutionContext, filename string) string {
	accountID := "_anonymous"
	if execCtx.AccountID != nil {
		accountID = execCtx.AccountID.String()
	}
	return fmt.Sprintf("%s/%s/%s", accountID, execCtx.RunID.String(), filename)
}

func errResult(class, msg string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error:      &tools.ExecutionError{ErrorClass: class, Message: msg},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	d := time.Since(started)
	if d < 0 {
		return 0
	}
	return int(d / time.Millisecond)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
