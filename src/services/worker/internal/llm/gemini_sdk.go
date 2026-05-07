package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/genai"
)

type geminiSDKGateway struct {
	cfg       GeminiGatewayConfig
	transport protocolTransport
	protocol  GeminiProtocolConfig
	client    *genai.Client
	configErr error
}

func NewGeminiGatewaySDK(cfg GeminiGatewayConfig) Gateway {
	transport := cfg.Transport
	if strings.TrimSpace(transport.APIKey) == "" {
		transport.APIKey = cfg.APIKey
	}
	if strings.TrimSpace(transport.BaseURL) == "" {
		transport.BaseURL = cfg.BaseURL
	}
	if transport.TotalTimeout <= 0 {
		transport.TotalTimeout = cfg.TotalTimeout
	}
	if !transport.EmitDebugEvents {
		transport.EmitDebugEvents = cfg.Transport.EmitDebugEvents
	}

	protocol := cfg.Protocol
	var configErr error
	if protocol.APIVersion == "" && len(protocol.AdvancedPayloadJSON) == 0 {
		protocol, configErr = parseGeminiProtocolConfig(cfg.AdvancedJSON)
	}
	if inferredVersion := geminiAPIVersionFromBaseURL(transport.BaseURL); inferredVersion != "" {
		protocol.APIVersion = inferredVersion
	}
	if strings.TrimSpace(protocol.APIVersion) == "" {
		protocol.APIVersion = "v1beta"
	}
	transport.BaseURL = normalizeGeminiBaseURL(transport.BaseURL)

	normalizedTransport := newProtocolTransport(transport, "https://generativelanguage.googleapis.com", nil)
	cfg.Transport = normalizedTransport.cfg
	cfg.Protocol = protocol
	cfg.TotalTimeout = normalizedTransport.cfg.TotalTimeout
	cfg.BaseURL = normalizedTransport.cfg.BaseURL

	headers := http.Header{}
	for key, value := range normalizedTransport.cfg.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			headers.Set(key, value)
		}
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:    genai.BackendGeminiAPI,
		APIKey:     strings.TrimSpace(normalizedTransport.cfg.APIKey),
		HTTPClient: sdkHTTPClient(normalizedTransport),
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    sdkBaseURL(normalizedTransport),
			APIVersion: protocol.APIVersion,
			Headers:    headers,
		},
	})
	if err != nil && configErr == nil {
		configErr = err
	}

	return &geminiSDKGateway{cfg: cfg, transport: normalizedTransport, protocol: protocol, client: client, configErr: configErr}
}

func (g *geminiSDKGateway) ProtocolKind() ProtocolKind { return ProtocolKindGeminiGenerateContent }

func (g *geminiSDKGateway) Stream(ctx context.Context, request Request, yield func(StreamEvent) error) error {
	if g.configErr != nil {
		return yield(StreamRunFailed{Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: g.configErr.Error()}})
	}
	if g.transport.baseURLErr != nil {
		return yield(StreamRunFailed{Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: "Gemini base_url blocked", Details: map[string]any{"reason": g.transport.baseURLErr.Error()}}})
	}
	ctx, stopTimeout, markActivity := withStreamIdleTimeout(ctx, g.transport.cfg.TotalTimeout)
	defer stopTimeout()
	llmCallID := uuid.NewString()
	PrepareRequestModelInputImages(&request)

	payload, err := toGeminiPayload(request, g.protocol.AdvancedPayloadJSON)
	if err != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: "Gemini payload construction failed", Details: map[string]any{"reason": err.Error()}}})
	}
	contents, config, err := geminiSDKRequest(payload)
	if err != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: "Gemini payload construction failed", Details: map[string]any{"reason": err.Error()}}})
	}

	path := geminiVersionedPath(g.transport.cfg.BaseURL, g.protocol.APIVersion, fmt.Sprintf("/models/%s:streamGenerateContent", request.Model))
	requestEvent, payloadBytes, err := g.requestEvent(request, llmCallID, path, payload)
	if err != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: "Gemini request serialization failed"}})
	}
	if RequestPayloadTooLarge(payloadBytes) {
		if err := yield(requestEvent); err != nil {
			return err
		}
		return yield(PreflightOversizeFailure(llmCallID, payloadBytes))
	}
	*requestEvent.NetworkAttempted = true
	if err := yield(requestEvent); err != nil {
		return err
	}

	state := newGeminiSDKStreamState(ctx, llmCallID, yield)
	for response, err := range g.client.Models.GenerateContentStream(ctx, request.Model, contents, config) {
		if err != nil {
			if emitErr := g.emitDebugErrorChunk(llmCallID, err, yield); emitErr != nil {
				return emitErr
			}
			return state.fail(geminiSDKErrorToGateway(err, payloadBytes))
		}
		markActivity()
		if err := g.emitDebugChunk(llmCallID, response, nil, yield); err != nil {
			return err
		}
		if err := state.handle(response); err != nil {
			return err
		}
	}
	return state.complete()
}

func (g *geminiSDKGateway) requestEvent(request Request, llmCallID string, path string, payload map[string]any) (StreamLlmRequest, int, error) {
	debugPayload, redactedHints := sanitizeDebugPayloadJSON(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return StreamLlmRequest{}, 0, err
	}
	baseURL := g.transport.cfg.BaseURL
	stats := ComputeRequestStats(request)
	networkAttempted := false
	return StreamLlmRequest{LlmCallID: llmCallID, ProviderKind: "gemini", APIMode: "generate_content", BaseURL: &baseURL, Path: &path, InputJSON: request.ToJSON(), PayloadJSON: debugPayload, RedactedHints: redactedHints, SystemBytes: stats.SystemBytes, ToolsBytes: stats.ToolsBytes, MessagesBytes: stats.MessagesBytes, AbstractRequestBytes: stats.AbstractRequestBytes, ProviderPayloadBytes: len(encoded), ImagePartCount: stats.ImagePartCount, Base64ImageBytes: stats.Base64ImageBytes, NetworkAttempted: &networkAttempted, RoleBytes: stats.RoleBytes, ToolSchemaBytesMap: stats.ToolSchemaBytesMap, StablePrefixHash: stats.StablePrefixHash, SessionPrefixHash: stats.SessionPrefixHash, VolatileTailHash: stats.VolatileTailHash, ToolSchemaHash: stats.ToolSchemaHash, StablePrefixBytes: stats.StablePrefixBytes, SessionPrefixBytes: stats.SessionPrefixBytes, VolatileTailBytes: stats.VolatileTailBytes, CacheCandidateBytes: stats.CacheCandidateBytes}, len(encoded), nil
}

func geminiSDKRequest(payload map[string]any) ([]*genai.Content, *genai.GenerateContentConfig, error) {
	contents, err := geminiSDKContents(payload["contents"])
	if err != nil {
		return nil, nil, err
	}
	config, err := geminiSDKGenerateContentConfig(payload)
	if err != nil {
		return nil, nil, err
	}
	if system, ok := payload["systemInstruction"].(map[string]any); ok {
		config.SystemInstruction, _ = geminiSDKContent(system)
	}
	return contents, config, nil
}

func geminiSDKGenerateContentConfig(payload map[string]any) (*genai.GenerateContentConfig, error) {
	config := &genai.GenerateContentConfig{}
	extraBody := copyAnyMap(payload)
	for _, key := range []string{"contents", "systemInstruction", "generationConfig", "tools", "toolConfig"} {
		delete(extraBody, key)
	}
	if len(extraBody) > 0 {
		config.HTTPOptions = &genai.HTTPOptions{ExtraBody: extraBody}
	}
	if generationConfig, ok := payload["generationConfig"].(map[string]any); ok {
		if err := genai.InternalMapToStruct(generationConfig, config); err != nil {
			return nil, err
		}
	}
	if rawTools, ok := payload["tools"].([]map[string]any); ok {
		tools, err := geminiSDKTools(rawTools)
		if err != nil {
			return nil, err
		}
		config.Tools = tools
	}
	if rawToolConfig, ok := payload["toolConfig"].(map[string]any); ok {
		toolConfig, err := geminiSDKToolConfig(rawToolConfig)
		if err != nil {
			return nil, err
		}
		config.ToolConfig = toolConfig
	}
	return config, nil
}

func geminiSDKContents(raw any) ([]*genai.Content, error) {
	arr, ok := raw.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("contents must be array")
	}
	out := make([]*genai.Content, 0, len(arr))
	for _, item := range arr {
		content, err := geminiSDKContent(item)
		if err != nil {
			return nil, err
		}
		out = append(out, content)
	}
	return out, nil
}
func geminiSDKContent(item map[string]any) (*genai.Content, error) {
	role, _ := item["role"].(string)
	rawParts, _ := item["parts"].([]map[string]any)
	parts := make([]*genai.Part, 0, len(rawParts))
	for _, raw := range rawParts {
		part, err := geminiSDKPart(raw)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return &genai.Content{Role: role, Parts: parts}, nil
}
func geminiSDKPart(raw map[string]any) (*genai.Part, error) {
	if text, ok := raw["text"].(string); ok {
		return &genai.Part{Text: text}, nil
	}
	if inlineData, ok := raw["inlineData"].(map[string]any); ok {
		data, _ := inlineData["data"].(string)
		mime, _ := inlineData["mimeType"].(string)
		decoded, err := decodeBase64String(data)
		if err != nil {
			return nil, err
		}
		return &genai.Part{InlineData: &genai.Blob{MIMEType: mime, Data: decoded}}, nil
	}
	if fc, ok := raw["functionCall"].(map[string]any); ok {
		id, _ := fc["id"].(string)
		name, _ := fc["name"].(string)
		args, _ := fc["args"].(map[string]any)
		return &genai.Part{FunctionCall: &genai.FunctionCall{ID: strings.TrimSpace(id), Name: name, Args: mapOrEmpty(args)}}, nil
	}
	if fr, ok := raw["functionResponse"].(map[string]any); ok {
		id, _ := fr["id"].(string)
		name, _ := fr["name"].(string)
		return &genai.Part{FunctionResponse: &genai.FunctionResponse{ID: strings.TrimSpace(id), Name: name, Response: geminiSDKFunctionResponseMap(fr["response"])}}, nil
	}
	return &genai.Part{Text: ""}, nil
}

func geminiSDKFunctionResponseMap(response any) map[string]any {
	if obj, ok := response.(map[string]any); ok {
		return mapOrEmpty(obj)
	}
	if response == nil {
		return map[string]any{}
	}
	return map[string]any{"output": response}
}

func geminiSDKTools(rawTools []map[string]any) ([]*genai.Tool, error) {
	tools := make([]*genai.Tool, 0, len(rawTools))
	for _, rawTool := range rawTools {
		rawDecls, _ := rawTool["functionDeclarations"].([]map[string]any)
		if len(rawDecls) == 0 {
			continue
		}
		tool := &genai.Tool{FunctionDeclarations: make([]*genai.FunctionDeclaration, 0, len(rawDecls))}
		for _, rawDecl := range rawDecls {
			decl := &genai.FunctionDeclaration{}
			if name, _ := rawDecl["name"].(string); strings.TrimSpace(name) != "" {
				decl.Name = strings.TrimSpace(name)
			}
			if description, _ := rawDecl["description"].(string); strings.TrimSpace(description) != "" {
				decl.Description = description
			}
			if schema, ok := rawDecl["parametersJsonSchema"].(map[string]any); ok && len(schema) > 0 {
				decl.ParametersJsonSchema = schema
			}
			tool.FunctionDeclarations = append(tool.FunctionDeclarations, decl)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func geminiSDKToolConfig(raw map[string]any) (*genai.ToolConfig, error) {
	config := &genai.ToolConfig{}
	if err := genai.InternalMapToStruct(raw, config); err != nil {
		return nil, err
	}
	return config, nil
}

func decodeBase64String(data string) ([]byte, error) {
	var decoded []byte
	if err := json.Unmarshal([]byte(`"`+strings.TrimSpace(data)+`"`), &decoded); err == nil {
		return decoded, nil
	}
	return base64StdDecode(data)
}
func base64StdDecode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(data))
}

type geminiSDKStreamState struct {
	ctx               context.Context
	llmCallID         string
	yield             func(StreamEvent) error
	usage             *Usage
	emitted           bool
	completed         bool
	contentPartCount  int
	thinkingPartCount int
	visibleTextLen    int
	toolCallCount     int
	toolBuffers       map[string]ToolCall
	toolOrder         []string
}

func newGeminiSDKStreamState(ctx context.Context, id string, yield func(StreamEvent) error) *geminiSDKStreamState {
	return &geminiSDKStreamState{ctx: ctx, llmCallID: id, yield: yield, toolBuffers: map[string]ToolCall{}}
}
func (s *geminiSDKStreamState) handle(response *genai.GenerateContentResponse) error {
	if response == nil {
		return nil
	}
	s.usage = geminiSDKUsage(response.UsageMetadata)
	if len(response.Candidates) == 0 {
		if failure := geminiPromptFeedbackFailure(response.PromptFeedback); failure != nil {
			return s.fail(*failure)
		}
		return nil
	}
	for _, candidate := range response.Candidates {
		if failure := geminiFinishReasonFailure(string(candidate.FinishReason)); failure != nil {
			return s.fail(*failure)
		}
		if candidate.Content == nil {
			continue
		}
		for partIndex, part := range candidate.Content.Parts {
			if part == nil {
				continue
			}
			if part.Text != "" {
				s.contentPartCount++
				if part.Thought {
					s.thinkingPartCount++
					ch := "thinking"
					if err := s.yield(StreamMessageDelta{ContentDelta: part.Text, Role: "assistant", Channel: &ch}); err != nil {
						return err
					}
				} else {
					s.visibleTextLen += len(part.Text)
					s.emitted = true
					if err := s.yield(StreamMessageDelta{ContentDelta: part.Text, Role: "assistant"}); err != nil {
						return err
					}
				}
			}
			if part.FunctionCall != nil {
				s.emitted = true
				s.bufferToolCall(part.FunctionCall, partIndex)
			}
		}
		if isGeminiTerminalFinishReason(string(candidate.FinishReason)) {
			if err := s.flushToolCalls(); err != nil {
				return err
			}
			s.completed = true
		}
	}
	return nil
}

func (s *geminiSDKStreamState) bufferToolCall(functionCall *genai.FunctionCall, partIndex int) {
	toolCallID := strings.TrimSpace(functionCall.ID)
	if toolCallID == "" {
		toolCallID = fmt.Sprintf("%s:%d", s.llmCallID, partIndex)
	}
	if _, exists := s.toolBuffers[toolCallID]; !exists {
		s.toolOrder = append(s.toolOrder, toolCallID)
		s.toolCallCount++
	}
	s.toolBuffers[toolCallID] = ToolCall{ToolCallID: toolCallID, ToolName: CanonicalToolName(functionCall.Name), ArgumentsJSON: mapOrEmpty(functionCall.Args)}
}

func (s *geminiSDKStreamState) flushToolCalls() error {
	for _, toolCallID := range s.toolOrder {
		call := s.toolBuffers[toolCallID]
		if err := s.yield(call); err != nil {
			return err
		}
	}
	s.toolBuffers = map[string]ToolCall{}
	s.toolOrder = nil
	return nil
}

func isGeminiTerminalFinishReason(finishReason string) bool {
	reason := strings.TrimSpace(finishReason)
	return reason == "STOP" || reason == "MAX_TOKENS"
}

func (s *geminiSDKStreamState) fail(g GatewayError) error {
	return s.yield(StreamRunFailed{LlmCallID: s.llmCallID, Error: g, Usage: s.usage})
}
func (s *geminiSDKStreamState) complete() error {
	if len(s.toolOrder) > 0 {
		return s.fail(GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "Gemini stream ended before tool call completion"})
	}
	if s.completed || s.emitted || s.usage != nil {
		logProviderCompletionDebug(s.ctx, providerCompletionDebug{
			ProviderKind:      "gemini",
			APIMode:           "generate_content",
			LlmCallID:         s.llmCallID,
			ContentPartCount:  s.contentPartCount,
			ThinkingPartCount: s.thinkingPartCount,
			VisibleTextLen:    s.visibleTextLen,
			ToolCallCount:     s.toolCallCount,
		})
		return s.yield(StreamRunCompleted{LlmCallID: s.llmCallID, Usage: s.usage})
	}
	return s.yield(StreamRunFailed{LlmCallID: s.llmCallID, Error: RetryableStreamEndedError()})
}
func geminiSDKUsage(meta *genai.GenerateContentResponseUsageMetadata) *Usage {
	if meta == nil {
		return nil
	}
	return parseGeminiUsage(&geminiUsageMetadata{PromptTokenCount: int(meta.PromptTokenCount), CandidatesTokenCount: int(meta.CandidatesTokenCount), TotalTokenCount: int(meta.TotalTokenCount), CachedContentTokenCount: int(meta.CachedContentTokenCount)})
}

func (g *geminiSDKGateway) emitDebugChunk(llmCallID string, response *genai.GenerateContentResponse, statusCode *int, yield func(StreamEvent) error) error {
	if !g.transport.cfg.EmitDebugEvents || response == nil {
		return nil
	}
	rawBytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	raw, truncated := truncateUTF8(string(rawBytes), geminiMaxDebugChunkBytes)
	var chunkJSON any
	_ = json.Unmarshal([]byte(raw), &chunkJSON)
	return yield(StreamLlmResponseChunk{LlmCallID: llmCallID, ProviderKind: "gemini", APIMode: "generate_content", Raw: raw, ChunkJSON: chunkJSON, StatusCode: statusCode, Truncated: truncated})
}

func (g *geminiSDKGateway) emitDebugErrorChunk(llmCallID string, err error, yield func(StreamEvent) error) error {
	var apiErr genai.APIError
	if !errors.As(err, &apiErr) {
		return nil
	}
	status := apiErr.Code
	raw := apiErr.Message
	if raw == "" {
		raw = err.Error()
	}
	truncatedRaw, truncated := truncateUTF8(raw, geminiMaxDebugChunkBytes)
	return yield(StreamLlmResponseChunk{LlmCallID: llmCallID, ProviderKind: "gemini", APIMode: "generate_content", Raw: truncatedRaw, StatusCode: &status, Truncated: truncated})
}

func geminiSDKGenerateImageConfig(advancedJSON map[string]any) (*genai.GenerateContentConfig, error) {
	payload := copyAnyMap(advancedJSON)
	generationConfig := map[string]any{"responseModalities": []any{"IMAGE"}}
	if raw, ok := payload["generationConfig"].(map[string]any); ok {
		for key, value := range raw {
			if key == "responseModalities" {
				continue
			}
			generationConfig[key] = value
		}
	}
	payload["generationConfig"] = generationConfig
	return geminiSDKGenerateContentConfig(payload)
}

func geminiSDKErrorToGateway(err error, payloadBytes int) GatewayError {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		details := map[string]any{"status_code": apiErr.Code}
		if apiErr.Status != "" {
			details["gemini_error_status"] = apiErr.Status
		}
		if apiErr.Code == http.StatusRequestEntityTooLarge {
			details["network_attempted"] = true
			details = OversizeFailureDetails(payloadBytes, OversizePhaseProvider, details)
		}
		message := strings.TrimSpace(apiErr.Message)
		if message == "" {
			message = "Gemini request failed"
		}
		return GatewayError{ErrorClass: classifyHTTPStatus(apiErr.Code), Message: message, Details: details}
	}
	return GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "Gemini network error", Details: map[string]any{"reason": err.Error()}}
}

func (g *geminiSDKGateway) GenerateImage(ctx context.Context, model string, req ImageGenerationRequest) (GeneratedImage, error) {
	if g == nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassConfigMissing, Message: "gemini gateway is not initialized"}
	}
	if g.configErr != nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: g.configErr.Error()}
	}
	if g.transport.baseURLErr != nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "Gemini base_url blocked", Details: map[string]any{"reason": g.transport.baseURLErr.Error()}}
	}
	if shouldUseVertexImageGeneration(g.transport.cfg.BaseURL, model) {
		return g.generateImageWithVertex(ctx, model, req)
	}
	parts, err := imageGenerationGeminiParts(req)
	if err != nil {
		return GeneratedImage{}, err
	}
	content, err := geminiSDKContent(map[string]any{"role": "user", "parts": parts})
	if err != nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "image input encoding failed", Details: map[string]any{"reason": err.Error()}}
	}
	config, err := geminiSDKGenerateImageConfig(g.protocol.AdvancedPayloadJSON)
	if err != nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "Gemini image config construction failed", Details: map[string]any{"reason": err.Error()}}
	}
	response, err := g.client.Models.GenerateContent(ctx, strings.TrimSpace(model), []*genai.Content{content}, config)
	if err != nil {
		return GeneratedImage{}, geminiSDKErrorToGateway(err, 0)
	}
	for _, candidate := range response.Candidates {
		if candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part != nil && part.InlineData != nil && len(part.InlineData.Data) > 0 {
				mimeType := strings.TrimSpace(part.InlineData.MIMEType)
				if mimeType == "" {
					mimeType = detectGeneratedImageMime(part.InlineData.Data)
				}
				if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
					return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "Gemini returned non-image content for image generation", Details: map[string]any{"mime_type": mimeType}}
				}
				return GeneratedImage{Bytes: part.InlineData.Data, MimeType: mimeType, ProviderKind: "gemini", Model: model}, nil
			}
		}
	}
	return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "Gemini image response contained no generated image"}
}

func (g *geminiSDKGateway) generateImageWithVertex(ctx context.Context, model string, req ImageGenerationRequest) (GeneratedImage, error) {
	if strings.Contains(strings.ToLower(strings.TrimSpace(g.transport.cfg.BaseURL)), "zenmux.ai") && len(req.InputImages) == 0 {
		return g.generateImageWithZenMuxVertexPredict(ctx, model, req)
	}
	client, err := g.vertexClient(ctx)
	if err != nil {
		return GeneratedImage{}, err
	}
	config := vertexImageGenerationConfig(req)
	var image *genai.GeneratedImage
	if len(req.InputImages) > 0 {
		refs, err := vertexReferenceImages(req.InputImages)
		if err != nil {
			return GeneratedImage{}, err
		}
		response, err := client.Models.EditImage(ctx, strings.TrimSpace(model), req.Prompt, refs, &genai.EditImageConfig{HTTPOptions: config.HTTPOptions})
		if err != nil {
			return GeneratedImage{}, geminiSDKErrorToGateway(err, 0)
		}
		if len(response.GeneratedImages) > 0 {
			image = response.GeneratedImages[0]
		}
	} else {
		response, err := client.Models.GenerateImages(ctx, strings.TrimSpace(model), req.Prompt, config)
		if err != nil {
			return GeneratedImage{}, geminiSDKErrorToGateway(err, 0)
		}
		if len(response.GeneratedImages) > 0 {
			image = response.GeneratedImages[0]
		}
	}
	if image == nil || image.Image == nil || len(image.Image.ImageBytes) == 0 {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "Vertex image response contained no generated image"}
	}
	mimeType := strings.TrimSpace(image.Image.MIMEType)
	if mimeType == "" {
		mimeType = detectGeneratedImageMime(image.Image.ImageBytes)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "Vertex returned non-image content for image generation", Details: map[string]any{"mime_type": mimeType}}
	}
	return GeneratedImage{
		Bytes:         image.Image.ImageBytes,
		MimeType:      mimeType,
		ProviderKind:  "zenmux",
		Model:         strings.TrimSpace(model),
		RevisedPrompt: strings.TrimSpace(image.EnhancedPrompt),
	}, nil
}

func (g *geminiSDKGateway) generateImageWithZenMuxVertexPredict(ctx context.Context, model string, req ImageGenerationRequest) (GeneratedImage, error) {
	provider, modelID, ok := splitVertexPublisherModel(model)
	if !ok {
		return GeneratedImage{}, GatewayError{
			ErrorClass: ErrorClassConfigInvalid,
			Message:    "ZenMux Vertex image model must be formatted as provider/model",
			Details:    map[string]any{"model": strings.TrimSpace(model)},
		}
	}

	payload := map[string]any{
		"instances": []map[string]any{{"prompt": req.Prompt}},
		"parameters": map[string]any{
			"sampleCount": 1,
		},
	}
	params := payload["parameters"].(map[string]any)
	if req.Size != "" {
		if strings.Contains(req.Size, ":") {
			params["aspectRatio"] = req.Size
		} else {
			params["imageSize"] = req.Size
		}
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}
	if req.OutputFormat != "" {
		format := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(req.OutputFormat)), "image/")
		params["outputOptions"] = map[string]any{"mimeType": "image/" + format}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassInternalError, Message: "ZenMux Vertex image request encoding failed", Details: map[string]any{"reason": err.Error()}}
	}

	endpoint := g.transport.endpoint(fmt.Sprintf(
		"/v1/publishers/%s/models/%s:predict",
		url.PathEscape(provider),
		url.PathEscape(modelID),
	))
	var lastErr error
	for attempt := 0; attempt < zenMuxVertexImageMaxAttempts; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if err != nil {
			return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassInternalError, Message: "ZenMux Vertex image request construction failed", Details: map[string]any{"reason": err.Error()}}
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if apiKey := strings.TrimSpace(g.transport.cfg.APIKey); apiKey != "" {
			httpReq.Header.Set("x-goog-api-key", apiKey)
		}
		g.transport.applyDefaultHeaders(httpReq)

		resp, err := g.transport.client.Do(httpReq)
		if err != nil {
			lastErr = GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "ZenMux Vertex image request failed", Details: mergeContextErrorDetails(map[string]any{"reason": err.Error(), "provider_kind": "gemini", "api_mode": "vertex_predict", "network_attempted": true, "attempt": attempt + 1}, err, ctx)}
			if !sleepBeforeZenMuxVertexRetry(ctx, attempt) {
				return GeneratedImage{}, lastErr
			}
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, imageGenerationMaxResponseBytes+1))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "ZenMux Vertex image response read failed", Details: map[string]any{"reason": readErr.Error(), "status_code": resp.StatusCode, "attempt": attempt + 1}}
			if !sleepBeforeZenMuxVertexRetry(ctx, attempt) {
				return GeneratedImage{}, lastErr
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			truncated := len(body) > geminiMaxErrorBodyBytes
			if truncated {
				body = body[:geminiMaxErrorBodyBytes]
			}
			message, details := geminiErrorMessageAndDetails(body, resp.StatusCode, truncated)
			details["api_mode"] = "vertex_predict"
			details["attempt"] = attempt + 1
			lastErr = GatewayError{ErrorClass: errorClassFromStatus(resp.StatusCode), Message: message, Details: details}
			if isRetryableZenMuxVertexStatus(resp.StatusCode) && sleepBeforeZenMuxVertexRetry(ctx, attempt) {
				continue
			}
			return GeneratedImage{}, lastErr
		}
		if len(body) > imageGenerationMaxResponseBytes {
			return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "ZenMux Vertex image response too large", Details: map[string]any{"status_code": resp.StatusCode}}
		}
		return parseZenMuxVertexPredictImage(body, strings.TrimSpace(model))
	}
	if lastErr != nil {
		return GeneratedImage{}, lastErr
	}
	return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "ZenMux Vertex image request failed"}
}

const zenMuxVertexImageMaxAttempts = 3

func isRetryableZenMuxVertexStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func sleepBeforeZenMuxVertexRetry(ctx context.Context, attempt int) bool {
	if attempt >= zenMuxVertexImageMaxAttempts-1 {
		return false
	}
	delay := time.Duration(750*(attempt+1)) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func splitVertexPublisherModel(model string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(model), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func parseZenMuxVertexPredictImage(body []byte, model string) (GeneratedImage, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "ZenMux Vertex image response parse failed", Details: map[string]any{"reason": err.Error(), "response_excerpt": compactResponseExcerpt(body)}}
	}
	predictions, ok := root["predictions"].([]any)
	if !ok || len(predictions) == 0 {
		return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "ZenMux Vertex image response missing predictions", Details: map[string]any{"response_excerpt": compactResponseExcerpt(body)}}
	}
	for _, raw := range predictions {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		imageBase64 := strings.TrimSpace(stringValueFromAny(item["bytesBase64Encoded"]))
		if imageBase64 == "" {
			if imageObj, ok := item["image"].(map[string]any); ok {
				imageBase64 = strings.TrimSpace(stringValueFromAny(imageObj["bytesBase64Encoded"]))
			}
		}
		if imageBase64 == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(imageBase64)
		if err != nil {
			return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "ZenMux Vertex image decode failed", Details: map[string]any{"reason": err.Error()}}
		}
		mimeType := strings.TrimSpace(stringValueFromAny(item["mimeType"]))
		if mimeType == "" {
			mimeType = detectGeneratedImageMime(decoded)
		}
		return GeneratedImage{
			Bytes:         decoded,
			MimeType:      mimeType,
			ProviderKind:  "zenmux",
			Model:         strings.TrimSpace(model),
			RevisedPrompt: strings.TrimSpace(stringValueFromAny(item["prompt"])),
		}, nil
	}
	return GeneratedImage{}, GatewayError{ErrorClass: ErrorClassProviderNonRetryable, Message: "ZenMux Vertex image response contained no generated image", Details: map[string]any{"response_excerpt": compactResponseExcerpt(body)}}
}

func (g *geminiSDKGateway) vertexClient(ctx context.Context) (*genai.Client, error) {
	if g == nil {
		return nil, GatewayError{ErrorClass: ErrorClassConfigMissing, Message: "gemini gateway is not initialized"}
	}
	headers := http.Header{}
	for key, value := range g.transport.cfg.DefaultHeaders {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			headers.Set(key, value)
		}
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:    genai.BackendVertexAI,
		APIKey:     strings.TrimSpace(g.transport.cfg.APIKey),
		HTTPClient: sdkHTTPClient(g.transport),
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    sdkBaseURL(g.transport),
			APIVersion: strings.TrimSpace(g.protocol.APIVersion),
			Headers:    headers,
		},
	})
	if err != nil {
		return nil, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: err.Error()}
	}
	return client, nil
}

func shouldUseVertexImageGeneration(baseURL string, model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(baseURL))
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(normalized, "zenmux.ai") || strings.HasPrefix(model, "openai/") || strings.HasPrefix(model, "qwen/")
}

func vertexImageGenerationConfig(req ImageGenerationRequest) *genai.GenerateImagesConfig {
	cfg := &genai.GenerateImagesConfig{NumberOfImages: 1}
	if req.Size != "" {
		if strings.Contains(req.Size, ":") {
			cfg.AspectRatio = req.Size
		} else {
			cfg.ImageSize = req.Size
		}
	}
	if req.OutputFormat != "" {
		format := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(req.OutputFormat)), "image/")
		cfg.OutputMIMEType = "image/" + format
	}
	return cfg
}

func vertexReferenceImages(parts []ContentPart) ([]genai.ReferenceImage, error) {
	out := make([]genai.ReferenceImage, 0, len(parts))
	for idx, part := range parts {
		mimeType, data, err := modelInputImage(part)
		if err != nil {
			return nil, GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "Vertex image input encoding failed", Details: map[string]any{"index": idx, "reason": err.Error()}}
		}
		out = append(out, genai.NewRawReferenceImage(&genai.Image{ImageBytes: data, MIMEType: mimeType}, int32(idx+1)))
	}
	return out, nil
}
