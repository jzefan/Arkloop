package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/genai"
)

const (
	defaultVideoPollInterval        = 15 * time.Second
	defaultVideoInitialPollInterval = 5 * time.Second
	defaultVideoMaxPolls            = 40
)

type GeneratedVideo struct {
	Bytes        []byte
	MimeType     string
	ProviderKind string
	Model        string
	URI          string
	// Download, when non-nil, streams video bytes from the provider URI.
	// The caller must close the returned ReadCloser. MimeType may be empty
	// when Download is set; in that case the string return value carries the
	// content-type from the HTTP response header.
	Download func(ctx context.Context) (io.ReadCloser, string, error)
}

type VideoGenerationRequest struct {
	Prompt           string
	AspectRatio      string
	Resolution       string
	NegativePrompt   string
	PersonGeneration string
	DurationSeconds  int32
	FPS              int32
	GenerateAudio    *bool
	PollInterval     time.Duration
	MaxPolls         int
	InputImage       *ContentPart
}

func GenerateVideoWithResolvedConfig(ctx context.Context, cfg ResolvedGatewayConfig, req VideoGenerationRequest) (GeneratedVideo, error) {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" && req.InputImage == nil {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "video generation prompt or input image is required"}
	}
	switch cfg.ProtocolKind {
	case ProtocolKindGeminiGenerateContent:
		if cfg.Gemini == nil {
			return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "missing gemini protocol config"}
		}
		gateway := NewGeminiGatewaySDK(GeminiGatewayConfig{Transport: cfg.Transport, Protocol: *cfg.Gemini}).(*geminiSDKGateway)
		return gateway.GenerateVideo(ctx, cfg.Model, req)
	default:
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: fmt.Sprintf("video generation unsupported for protocol: %s", cfg.ProtocolKind)}
	}
}

func (g *geminiSDKGateway) GenerateVideo(ctx context.Context, model string, req VideoGenerationRequest) (GeneratedVideo, error) {
	if g == nil {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigMissing, Message: "gemini gateway is not initialized"}
	}
	if g.configErr != nil {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: g.configErr.Error()}
	}
	if g.transport.baseURLErr != nil {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "Gemini base_url blocked", Details: map[string]any{"reason": g.transport.baseURLErr.Error()}}
	}
	client, err := g.vertexClient(ctx)
	if err != nil {
		return GeneratedVideo{}, err
	}
	source := &genai.GenerateVideosSource{Prompt: strings.TrimSpace(req.Prompt)}
	if req.InputImage != nil {
		mimeType, data, err := modelInputImage(*req.InputImage)
		if err != nil {
			return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "video input image encoding failed", Details: map[string]any{"reason": err.Error()}}
		}
		source.Image = &genai.Image{ImageBytes: data, MIMEType: mimeType}
	}
	operation, err := client.Models.GenerateVideosFromSource(ctx, strings.TrimSpace(model), source, vertexVideoGenerationConfig(req))
	if err != nil {
		return GeneratedVideo{}, geminiSDKErrorToGateway(err, 0)
	}
	pollInterval := req.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultVideoPollInterval
	}
	initialPollInterval := defaultVideoInitialPollInterval
	if pollInterval < initialPollInterval {
		initialPollInterval = pollInterval
	}
	maxPolls := req.MaxPolls
	if maxPolls <= 0 {
		maxPolls = defaultVideoMaxPolls
	}
	for polls := 0; operation != nil && !operation.Done && polls < maxPolls; polls++ {
		interval := pollInterval
		if polls == 0 {
			interval = initialPollInterval
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return GeneratedVideo{}, ctx.Err()
		case <-timer.C:
		}
		operation, err = client.Operations.GetVideosOperation(ctx, operation, nil)
		if err != nil {
			return GeneratedVideo{}, geminiSDKErrorToGateway(err, 0)
		}
	}
	if operation == nil {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "video generation returned no operation"}
	}
	if !operation.Done {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "video generation operation did not complete before polling limit", Details: map[string]any{"operation": operation.Name}}
	}
	if len(operation.Error) > 0 {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "video generation operation failed", Details: operation.Error}
	}
	return generatedVideoFromOperation(ctx, operation, strings.TrimSpace(model), g.transport.client, sdkBaseURL(g.transport))
}

func vertexVideoGenerationConfig(req VideoGenerationRequest) *genai.GenerateVideosConfig {
	cfg := &genai.GenerateVideosConfig{NumberOfVideos: 1}
	if req.AspectRatio != "" {
		cfg.AspectRatio = strings.TrimSpace(req.AspectRatio)
	}
	if req.Resolution != "" {
		cfg.Resolution = strings.TrimSpace(req.Resolution)
	}
	if req.NegativePrompt != "" {
		cfg.NegativePrompt = strings.TrimSpace(req.NegativePrompt)
	}
	if req.PersonGeneration != "" {
		cfg.PersonGeneration = strings.TrimSpace(req.PersonGeneration)
	}
	if req.DurationSeconds > 0 {
		cfg.DurationSeconds = &req.DurationSeconds
	}
	if req.FPS > 0 {
		cfg.FPS = &req.FPS
	}
	if req.GenerateAudio != nil {
		cfg.GenerateAudio = req.GenerateAudio
	}
	return cfg
}

func generatedVideoFromOperation(_ context.Context, operation *genai.GenerateVideosOperation, model string, client *http.Client, allowedBaseURL string) (GeneratedVideo, error) {
	if operation.Response == nil || len(operation.Response.GeneratedVideos) == 0 {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "video generation response contained no generated video"}
	}
	video := operation.Response.GeneratedVideos[0].Video
	if video == nil {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "video generation response contained empty video"}
	}

	uri := strings.TrimSpace(video.URI)
	mimeType := strings.TrimSpace(video.MIMEType)

	if len(video.VideoBytes) > 0 {
		if mimeType == "" {
			mimeType = http.DetectContentType(video.VideoBytes)
		}
		if !strings.HasPrefix(strings.ToLower(mimeType), "video/") {
			mimeType = "video/mp4"
		}
		return GeneratedVideo{Bytes: video.VideoBytes, MimeType: mimeType, ProviderKind: "zenmux", Model: model, URI: uri}, nil
	}

	if uri == "" {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "video generation returned no downloadable bytes or URI"}
	}
	if !isAllowedVideoHost(uri, allowedBaseURL) {
		return GeneratedVideo{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "video generation returned a URI from a disallowed host", Details: map[string]any{"uri": uri}}
	}

	capturedURI := uri
	capturedClient := client
	downloadFn := func(ctx context.Context) (io.ReadCloser, string, error) {
		return openVideoDownload(ctx, capturedClient, capturedURI, allowedBaseURL)
	}
	return GeneratedVideo{MimeType: mimeType, ProviderKind: "zenmux", Model: model, URI: uri, Download: downloadFn}, nil
}

// isAllowedVideoHost checks that uri's hostname is a known Google domain or matches the
// configured Gemini base URL host, preventing SSRF via provider-supplied video URIs.
func isAllowedVideoHost(rawURI, configuredBaseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURI))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range []string{".googleapis.com", ".google.com", ".ggpht.com"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	if configuredBaseURL != "" {
		if b, err := url.Parse(configuredBaseURL); err == nil {
			if strings.ToLower(b.Hostname()) == host {
				return true
			}
		}
	}
	return false
}

// openVideoDownload opens an HTTP GET to uri and returns the response body as a
// streaming ReadCloser. The caller is responsible for closing it. The second
// return value is the Content-Type from the response header (may be empty).
// SSRF validation must be done by the caller before invoking this function.
func openVideoDownload(ctx context.Context, client *http.Client, uri, allowedBaseURL string) (io.ReadCloser, string, error) {
	if !isAllowedVideoHost(uri, allowedBaseURL) {
		return nil, "", GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "video generation returned a URI from a disallowed host", Details: map[string]any{"uri": uri}}
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, "", GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "video generation returned an invalid URI", Details: map[string]any{"uri": uri, "reason": err.Error()}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "download generated video failed", Details: map[string]any{"uri": uri, "reason": err.Error()}}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_ = resp.Body.Close()
		return nil, "", GatewayError{ErrorClass: classifyHTTPStatus(resp.StatusCode), Message: fmt.Sprintf("download generated video failed with HTTP %d", resp.StatusCode), Details: map[string]any{"uri": uri, "status": resp.StatusCode}}
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	return resp.Body, contentType, nil
}
