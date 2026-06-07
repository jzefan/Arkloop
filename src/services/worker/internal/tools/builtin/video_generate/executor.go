// Package videogenerate implements the video_generate builtin tool.
//
// Currently targets Doubao Seedance (Volcengine ARK) via the asynchronous
// /api/v3/contents/generations/tasks endpoint:
//
//	1. POST creates a generation task → returns {id}
//	2. GET polls until status ∈ {succeeded, failed, cancelled}
//	3. On succeeded, content.video_url is a TOS pre-signed URL valid ~24h
//	4. Download mp4 bytes and persist as a run-scoped artifact
//
// The Seedance API is multimodal-content-list shaped (similar to OpenAI's
// chat completions multi-part content), with a text block carrying the prompt
// plus optional `image_url` parts whose `role="first_frame"` selects the
// starting frame for image-to-video.
package videogenerate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	videoGenerateConfigKey    = "video_generative.model"
	defaultGeneratedVideoName = "generated-video"
	defaultDurationSeconds    = 5
	maxDurationSeconds        = 12
	// Seedance 2.0-fast 实测不接受 duration=3（报 InvalidParameter）；
	// 最小公约数定 5，避免被上游拒。1.0-pro 支持 3-10，5 在所有版本都安全。
	minDurationSeconds = 5
	defaultResolution         = "480p"
	pollInterval              = 8 * time.Second
	pollTimeout               = 5 * time.Minute
	httpRequestTimeout        = 60 * time.Second
	videoDownloadTimeout      = 120 * time.Second
)

type ToolExecutor struct {
	store         objectstore.Store
	db            workerdata.QueryDB
	config        sharedconfig.Resolver
	routingLoader *routing.ConfigLoader
	// generate 是测试 stub 注入点；nil 时走真实 Seedance HTTP 客户端。
	generate func(ctx context.Context, resolved llm.ResolvedGatewayConfig, req GenerateRequest) (GeneratedVideo, error) //nolint:unused
}

func NewToolExecutor(
	store objectstore.Store,
	db workerdata.QueryDB,
	config sharedconfig.Resolver,
	routingLoader *routing.ConfigLoader,
) *ToolExecutor {
	return &ToolExecutor{
		store:         store,
		db:            db,
		config:        config,
		routingLoader: routingLoader,
	}
}

// GenerateRequest 抽象底层 provider 的入参（目前只支持 Seedance）。
type GenerateRequest struct {
	Prompt          string
	FirstFrameBytes []byte // optional, may be nil
	FirstFrameMime  string // e.g. "image/png"
	DurationSeconds int
	Resolution      string
}

// GeneratedVideo 抽象 provider 返回的视频负载。
type GeneratedVideo struct {
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
		return errResult("tool.not_configured", "video generation storage is not configured", started)
	}
	if execCtx.AccountID == nil {
		return errResult("tool.execution_failed", "account context is required", started)
	}

	prompt := strings.TrimSpace(stringArg(args, "prompt"))
	if prompt == "" {
		return errResult("tool.args_invalid", "parameter prompt is required", started)
	}

	firstFrameRef := strings.TrimSpace(stringArg(args, "first_frame"))
	var firstFrameBytes []byte
	var firstFrameMime string
	if firstFrameRef != "" {
		bytes, mime, err := e.loadFirstFrame(ctx, firstFrameRef, *execCtx.AccountID)
		if err != nil {
			return errResult("tool.args_invalid", err.Error(), started)
		}
		firstFrameBytes = bytes
		firstFrameMime = mime
	}

	duration := defaultDurationSeconds
	if raw, ok := args["duration_seconds"]; ok && raw != nil {
		switch v := raw.(type) {
		case int:
			duration = v
		case float64:
			duration = int(v)
		case string:
			if parsed, err := time.ParseDuration(v + "s"); err == nil {
				duration = int(parsed.Seconds())
			}
		}
	}
	if duration < minDurationSeconds {
		duration = minDurationSeconds
	}
	if duration > maxDurationSeconds {
		duration = maxDurationSeconds
	}

	resolution := strings.TrimSpace(stringArg(args, "resolution"))
	if resolution == "" {
		resolution = defaultResolution
	}

	explicitSelector := strings.TrimSpace(stringArg(args, "model_selector"))
	selected, err := e.resolveSelectedRoute(ctx, *execCtx.AccountID, explicitSelector)
	if err != nil {
		return errResult("tool.not_configured", err.Error(), started)
	}
	resolved, err := pipeline.ResolveGatewayConfigFromSelectedRouteForRequest(ctx, *selected, false, 0)
	if err != nil {
		return errResult("tool.execution_failed", fmt.Sprintf("resolve video model failed: %s", err.Error()), started)
	}

	request := GenerateRequest{
		Prompt:          prompt,
		FirstFrameBytes: firstFrameBytes,
		FirstFrameMime:  firstFrameMime,
		DurationSeconds: duration,
		Resolution:      resolution,
	}

	generator := e.generate
	if generator == nil {
		// 目前唯一支持的 provider：Doubao Seedance（火山引擎 ARK 异步任务）
		generator = generateVideoViaSeedance
	}
	video, err := generator(ctx, resolved, request)
	if err != nil {
		return errResultWithDetails("provider.non_retryable", err.Error(), errorDetails(err), started)
	}
	if len(video.Bytes) == 0 {
		return errResult("tool.execution_failed", "video provider returned empty bytes", started)
	}

	artifactBase := sanitizeArtifactName(strings.TrimSpace(stringArg(args, "artifact_name")))
	if artifactBase == "" {
		artifactBase = defaultGeneratedVideoName
	}
	contentType := video.MimeType
	if contentType == "" {
		contentType = "video/mp4"
	}
	filename := artifactBase + fileExtForContentType(contentType)
	key := buildArtifactKey(execCtx, filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		value := execCtx.ThreadID.String()
		threadID = &value
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), execCtx.AccountID.String(), threadID)
	if err := e.store.PutObject(ctx, key, video.Bytes, objectstore.PutOptions{
		ContentType: contentType,
		Metadata:    metadata,
	}); err != nil {
		return errResult("tool.upload_failed", fmt.Sprintf("save generated video failed: %s", err.Error()), started)
	}

	result := map[string]any{
		"provider":  video.ProviderKind,
		"model":     video.Model,
		"mime_type": contentType,
		"bytes":     len(video.Bytes),
		"duration":  duration,
		"resolution": resolution,
		"artifacts": []map[string]any{
			{
				"key":       key,
				"filename":  filename,
				"size":      len(video.Bytes),
				"mime_type": contentType,
				"title":     artifactBase,
				"display":   "inline",
			},
		},
	}
	return tools.ExecutionResult{
		ResultJSON: result,
		DurationMs: durationMs(started),
	}
}

// IsAvailableForAccount mirrors image_generate's logic: prefer an explicit
// account/system default selector; otherwise fall back to scanning the routing
// table for any route that exposes output_modalities=["video"].
func (e *ToolExecutor) IsAvailableForAccount(ctx context.Context, accountID uuid.UUID) bool {
	if accountID == uuid.Nil {
		slog.WarnContext(ctx, "video_generate.IsAvailableForAccount: nil account")
		return false
	}
	if _, err := e.resolveSelectedRoute(ctx, accountID, ""); err == nil {
		slog.InfoContext(ctx, "video_generate.IsAvailableForAccount: ok via configured selector", "account", accountID)
		return true
	} else {
		slog.InfoContext(ctx, "video_generate.IsAvailableForAccount: no explicit selector, falling back to route scan", "account", accountID, "err", err.Error())
	}
	if e.routingLoader == nil {
		slog.WarnContext(ctx, "video_generate.IsAvailableForAccount: routingLoader is nil")
		return false
	}
	cfg, err := e.routingLoader.Load(ctx, &accountID)
	if err != nil {
		slog.WarnContext(ctx, "video_generate.IsAvailableForAccount: routing load failed", "err", err.Error())
		return false
	}
	if len(cfg.Routes) == 0 {
		slog.WarnContext(ctx, "video_generate.IsAvailableForAccount: zero routes loaded")
		return false
	}
	var videoModels []string
	for _, route := range cfg.Routes {
		caps := routing.RouteModelCapabilities(route)
		if caps.SupportsOutputModality("video") {
			slog.InfoContext(ctx, "video_generate.IsAvailableForAccount: ok via fallback route", "model", route.Model)
			return true
		}
		if strings.Contains(strings.ToLower(route.Model), "video") || strings.Contains(strings.ToLower(route.Model), "seedance") {
			videoModels = append(videoModels, route.Model)
		}
	}
	slog.WarnContext(ctx, "video_generate.IsAvailableForAccount: no video route matched modality", "video_named_routes", videoModels, "total_routes", len(cfg.Routes))
	return false
}

func (e *ToolExecutor) IsAvailableForModelSelector(ctx context.Context, accountID uuid.UUID, selector string) error {
	if accountID == uuid.Nil {
		return fmt.Errorf("account context is required")
	}
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return fmt.Errorf("model selector must not be empty")
	}
	_, err := e.resolveSelectedRoute(ctx, accountID, selector)
	return err
}

func (e *ToolExecutor) resolveSelectedRoute(ctx context.Context, accountID uuid.UUID, explicitSelector string) (*routing.SelectedProviderRoute, error) {
	if e.routingLoader == nil {
		return nil, fmt.Errorf("video generation routing is not configured")
	}
	selector := strings.TrimSpace(explicitSelector)
	if selector == "" && e.db != nil {
		_ = e.db.QueryRow(ctx,
			`SELECT value FROM account_entitlement_overrides
			  WHERE account_id = $1 AND key = $2
			    AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
			  LIMIT 1`,
			accountID, videoGenerateConfigKey,
		).Scan(&selector)
		selector = strings.TrimSpace(selector)
	}
	if selector == "" && e.config != nil {
		if value, err := e.config.Resolve(ctx, videoGenerateConfigKey, sharedconfig.Scope{}); err == nil {
			selector = strings.TrimSpace(value)
		}
	}
	if selector == "" {
		return nil, fmt.Errorf("video generation model is not configured")
	}

	cfg, err := e.routingLoader.Load(ctx, &accountID)
	if err != nil {
		return nil, fmt.Errorf("load video routing config failed: %w", err)
	}
	if len(cfg.Routes) == 0 {
		return nil, fmt.Errorf("video routing config is empty")
	}
	credName, modelName, exact := splitModelSelector(selector)
	if exact {
		if route, cred, ok := cfg.GetHighestPriorityRouteByCredentialAndModel(credName, modelName, map[string]any{}); ok {
			return &routing.SelectedProviderRoute{Route: route, Credential: cred}, nil
		}
		return nil, fmt.Errorf("video generation route not found for selector: %s", selector)
	}
	if route, cred, ok := cfg.GetHighestPriorityRouteByModel(selector, map[string]any{}); ok {
		return &routing.SelectedProviderRoute{Route: route, Credential: cred}, nil
	}
	return nil, fmt.Errorf("video generation route not found for selector: %s", selector)
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

func (e *ToolExecutor) loadFirstFrame(ctx context.Context, ref string, accountID uuid.UUID) ([]byte, string, error) {
	key := strings.TrimSpace(ref)
	if strings.HasPrefix(key, "artifact:") {
		key = strings.TrimSpace(strings.TrimPrefix(key, "artifact:"))
	}
	if key == "" {
		return nil, "", fmt.Errorf("first_frame is empty")
	}
	if !strings.HasPrefix(key, accountID.String()+"/") {
		return nil, "", fmt.Errorf("first_frame is outside the current account")
	}
	data, contentType, err := e.store.GetWithContentType(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("first_frame not found: %w", err)
	}
	mime := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	return data, mime, nil
}

func buildArtifactKey(execCtx tools.ExecutionContext, filename string) string {
	accountID := "_anonymous"
	if execCtx.AccountID != nil {
		accountID = execCtx.AccountID.String()
	}
	return fmt.Sprintf("%s/%s/%s", accountID, execCtx.RunID.String(), filename)
}

// sanitizeArtifactName scrubs unsafe characters so the resulting key is path-safe.
// Mirrors image_generate's helper (kept here to avoid cross-package coupling).
func sanitizeArtifactName(raw string) string {
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-' || r == '_':
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

func fileExtForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	default:
		return ".mp4"
	}
}

func stringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

func errResult(errorClass, message string, started time.Time) tools.ExecutionResult {
	return errResultWithDetails(errorClass, message, nil, started)
}

func errResultWithDetails(errorClass, message string, details map[string]any, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorClass,
			Message:    message,
			Details:    copyMap(details),
		},
		DurationMs: durationMs(started),
	}
}

func errorDetails(err error) map[string]any {
	if seedErr, ok := err.(*seedanceError); ok {
		return map[string]any{
			"provider_kind":       "doubao",
			"status_code":         seedErr.StatusCode,
			"provider_error_body": seedErr.Body,
			"task_id":             seedErr.TaskID,
		}
	}
	return nil
}

func copyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func durationMs(started time.Time) int {
	elapsed := time.Since(started)
	if elapsed < 0 {
		return 0
	}
	return int(elapsed / time.Millisecond)
}

// ─────────────────── Seedance HTTP client ───────────────────

type seedanceError struct {
	StatusCode int
	Body       string
	TaskID     string
}

func (e *seedanceError) Error() string {
	if e.TaskID != "" {
		return fmt.Sprintf("seedance task %s failed (status %d): %s", e.TaskID, e.StatusCode, truncate(e.Body, 400))
	}
	return fmt.Sprintf("seedance request failed (status %d): %s", e.StatusCode, truncate(e.Body, 400))
}

type seedanceContentPart struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL map[string]any `json:"image_url,omitempty"`
	Role     string         `json:"role,omitempty"`
}

type seedanceTaskRequest struct {
	Model   string                `json:"model"`
	Content []seedanceContentPart `json:"content"`
}

type seedanceTaskResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// generateVideoViaSeedance 实现 Doubao Seedance 异步任务流程。
func generateVideoViaSeedance(ctx context.Context, resolved llm.ResolvedGatewayConfig, req GenerateRequest) (GeneratedVideo, error) {
	base := strings.TrimRight(strings.TrimSpace(resolved.Transport.BaseURL), "/")
	if base == "" {
		// 兜底：火山引擎 ARK 默认端点
		base = "https://ark.cn-beijing.volces.com/api/v3"
	}
	if !strings.HasSuffix(base, "/api/v3") && !strings.Contains(base, "/api/v") {
		base = base + "/api/v3"
	}
	if strings.TrimSpace(resolved.Transport.APIKey) == "" {
		return GeneratedVideo{}, fmt.Errorf("Doubao API key is empty")
	}

	// 1) 构造 content
	promptText := req.Prompt
	if req.DurationSeconds > 0 {
		promptText = fmt.Sprintf("%s --dur %d", promptText, req.DurationSeconds)
	}
	if req.Resolution != "" {
		promptText = fmt.Sprintf("%s --rs %s", promptText, req.Resolution)
	}
	parts := []seedanceContentPart{
		{Type: "text", Text: promptText},
	}
	if len(req.FirstFrameBytes) > 0 {
		mime := req.FirstFrameMime
		if mime == "" {
			mime = "image/png"
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(req.FirstFrameBytes))
		parts = append(parts, seedanceContentPart{
			Type:     "image_url",
			ImageURL: map[string]any{"url": dataURL},
			Role:     "first_frame",
		})
	}
	body, err := json.Marshal(seedanceTaskRequest{Model: resolved.Model, Content: parts})
	if err != nil {
		return GeneratedVideo{}, fmt.Errorf("marshal seedance request: %w", err)
	}

	// 2) 提交任务
	createEndpoint := base + "/contents/generations/tasks"
	taskID, err := submitSeedanceTask(ctx, createEndpoint, resolved.Transport.APIKey, body)
	if err != nil {
		return GeneratedVideo{}, err
	}

	// 3) 轮询
	videoURL, err := pollSeedanceTask(ctx, base, resolved.Transport.APIKey, taskID)
	if err != nil {
		return GeneratedVideo{}, err
	}

	// 4) 下载 mp4
	data, err := downloadVideo(ctx, videoURL)
	if err != nil {
		return GeneratedVideo{}, err
	}

	return GeneratedVideo{
		Bytes:        data,
		MimeType:     "video/mp4",
		ProviderKind: "doubao",
		Model:        resolved.Model,
	}, nil
}

func submitSeedanceTask(ctx context.Context, endpoint, apiKey string, body []byte) (string, error) {
	httpCtx, cancel := context.WithTimeout(ctx, httpRequestTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build seedance create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("seedance create call network error: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &seedanceError{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	var parsed seedanceTaskResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", &seedanceError{StatusCode: resp.StatusCode, Body: "create response not JSON: " + string(raw)}
	}
	if strings.TrimSpace(parsed.ID) == "" {
		return "", &seedanceError{StatusCode: resp.StatusCode, Body: "create response missing task id: " + string(raw)}
	}
	return parsed.ID, nil
}

func pollSeedanceTask(ctx context.Context, base, apiKey, taskID string) (string, error) {
	endpoint := base + "/contents/generations/tasks/" + taskID
	deadline := time.Now().Add(pollTimeout)
	for {
		if time.Now().After(deadline) {
			return "", &seedanceError{TaskID: taskID, Body: "polling timed out"}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		httpCtx, cancel := context.WithTimeout(ctx, httpRequestTimeout)
		httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			cancel()
			return "", fmt.Errorf("build seedance poll request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := http.DefaultClient.Do(httpReq)
		cancel()
		if err != nil {
			// 短暂网络错误，继续轮询直到 deadline
			time.Sleep(pollInterval)
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", &seedanceError{StatusCode: resp.StatusCode, TaskID: taskID, Body: string(raw)}
		}
		var parsed seedanceTaskResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", &seedanceError{StatusCode: resp.StatusCode, TaskID: taskID, Body: "poll response not JSON: " + string(raw)}
		}
		switch parsed.Status {
		case "succeeded":
			if strings.TrimSpace(parsed.Content.VideoURL) == "" {
				return "", &seedanceError{TaskID: taskID, Body: "succeeded but video_url is empty: " + string(raw)}
			}
			return parsed.Content.VideoURL, nil
		case "failed", "cancelled":
			msg := parsed.Status
			if parsed.Error != nil {
				msg = parsed.Status + ": " + parsed.Error.Message
			}
			return "", &seedanceError{TaskID: taskID, Body: msg}
		default:
			// queued / running → 继续
		}
		time.Sleep(pollInterval)
	}
}

func downloadVideo(ctx context.Context, url string) ([]byte, error) {
	httpCtx, cancel := context.WithTimeout(ctx, videoDownloadTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("download video network error: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, &seedanceError{StatusCode: resp.StatusCode, Body: "video download failed: " + truncate(string(raw), 200)}
	}
	return io.ReadAll(resp.Body)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
