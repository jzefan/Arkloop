package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	openAICodexResponsesProviderKind   = "openai_codex"
	openAICodexResponsesAPIMode        = "codex_responses"
	openAICodexResponsesPath           = "/responses"
	defaultOpenAICodexResponsesBaseURL = "https://chatgpt.com/backend-api/codex"
	openAICodexAuthClaimPath           = "https://api.openai.com/auth"
)

var openAICodexResponseStatuses = map[string]struct{}{
	"completed":   {},
	"incomplete":  {},
	"failed":      {},
	"cancelled":   {},
	"queued":      {},
	"in_progress": {},
}

type OpenAICodexResponsesGatewayConfig struct {
	Transport       TransportConfig
	Protocol        OpenAIProtocolConfig
	APIKey          string
	BaseURL         string
	EmitDebugEvents bool
	TotalTimeout    time.Duration
}

type openAICodexResponsesGateway struct {
	cfg       OpenAICodexResponsesGatewayConfig
	transport protocolTransport
	protocol  OpenAIProtocolConfig
	quirks    *QuirkStore
}

func NewOpenAICodexResponsesGateway(cfg OpenAICodexResponsesGatewayConfig) Gateway {
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
		transport.EmitDebugEvents = cfg.EmitDebugEvents
	}
	if transport.MaxResponseBytes <= 0 {
		transport.MaxResponseBytes = openAIMaxResponseBytes
	}

	protocol := cfg.Protocol
	protocol.PrimaryKind = ProtocolKindOpenAICodexResponses
	if protocol.AdvancedPayloadJSON == nil {
		protocol.AdvancedPayloadJSON = map[string]any{}
	}

	normalizedTransport := newProtocolTransport(transport, defaultOpenAICodexResponsesBaseURL, normalizeOpenAICodexResponsesBaseURL)
	cfg.Transport = normalizedTransport.cfg
	cfg.Protocol = protocol
	cfg.EmitDebugEvents = normalizedTransport.cfg.EmitDebugEvents
	cfg.TotalTimeout = normalizedTransport.cfg.TotalTimeout
	cfg.BaseURL = normalizedTransport.cfg.BaseURL

	return &openAICodexResponsesGateway{
		cfg:       cfg,
		transport: normalizedTransport,
		protocol:  protocol,
		quirks:    NewQuirkStore(),
	}
}

func (g *openAICodexResponsesGateway) ProtocolKind() ProtocolKind {
	return ProtocolKindOpenAICodexResponses
}

func (g *openAICodexResponsesGateway) Stream(ctx context.Context, request Request, yield func(StreamEvent) error) error {
	if g.transport.baseURLErr != nil {
		return yield(StreamRunFailed{Error: GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "Codex base_url blocked", Details: map[string]any{"reason": g.transport.baseURLErr.Error()}}})
	}
	ctx, stopTimeout, markActivity := withStreamIdleTimeout(ctx, g.transport.cfg.TotalTimeout)
	defer stopTimeout()
	return g.responses(ctx, request, yield, markActivity)
}

func (g *openAICodexResponsesGateway) responses(ctx context.Context, request Request, yield func(StreamEvent) error, markActivity func()) error {
	llmCallID := uuid.NewString()
	token, accountID, authErr := g.codexAuth()
	if authErr != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: *authErr})
	}

	payload, payloadBytes, requestEvent, err := g.responsesPayload(request, llmCallID)
	if err != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: "Codex responses input construction failed", Details: map[string]any{"reason": err.Error()}}})
	}
	if RequestPayloadTooLarge(payloadBytes) {
		if err := yield(requestEvent); err != nil {
			return err
		}
		return yield(PreflightOversizeFailure(llmCallID, payloadBytes))
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: GatewayError{ErrorClass: ErrorClassInternalError, Message: "Codex responses payload encode failed", Details: map[string]any{"reason": err.Error()}}})
	}

	responseCapture := newProviderResponseCapture()
	streamCtx := withProviderResponseCapture(ctx, responseCapture)
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, g.transport.endpoint(openAICodexResponsesPath), bytes.NewReader(encoded))
	if err != nil {
		return yield(StreamRunFailed{LlmCallID: llmCallID, Error: GatewayError{ErrorClass: ErrorClassConfigInvalid, Message: "Codex responses request construction failed", Details: map[string]any{"reason": err.Error()}}})
	}
	g.applyHeaders(httpReq, token, accountID)

	*requestEvent.NetworkAttempted = true
	if err := yield(requestEvent); err != nil {
		return err
	}

	state := newOpenAISDKResponsesState(ctx, llmCallID, yield)
	state.providerKind = openAICodexResponsesProviderKind
	state.apiMode = openAICodexResponsesAPIMode

	resp, err := g.transport.client.Do(httpReq)
	if err != nil {
		return state.fail(openAICodexStreamErrorToGateway(err, payloadBytes, responseCapture, streamCtx))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(openAIMaxErrorBodyBytes+1)))
		if readErr != nil {
			return state.fail(openAICodexStreamErrorToGateway(readErr, payloadBytes, responseCapture, streamCtx))
		}
		status := resp.StatusCode
		if err := g.emitDebugChunk(llmCallID, string(body), &status, yield); err != nil {
			return err
		}
		return state.fail(openAICodexHTTPError(resp, body, payloadBytes, responseCapture))
	}

	var handleErr error
	err = forEachSSEData(streamCtx, resp.Body, markActivity, func(data string) error {
		raw, ok := normalizeOpenAICodexSSEData(data)
		if !ok {
			return nil
		}
		if err := g.emitDebugChunk(llmCallID, raw, nil, yield); err != nil {
			handleErr = err
			return err
		}
		if err := state.handleRaw(raw); err != nil {
			handleErr = err
			return err
		}
		return nil
	})
	if err != nil {
		if handleErr != nil {
			return handleErr
		}
		return state.fail(openAICodexStreamErrorToGateway(err, payloadBytes, responseCapture, streamCtx))
	}
	return state.complete()
}

func (g *openAICodexResponsesGateway) responsesPayload(request Request, llmCallID string) (map[string]any, int, StreamLlmRequest, error) {
	payload, err := buildOpenAIResponsesPayload(request, g.protocol, g.quirks)
	if err != nil {
		return nil, 0, StreamLlmRequest{}, err
	}
	applyOpenAICodexResponsesPayloadDefaults(payload)
	return g.providerRequest(request, llmCallID, payload)
}

func (g *openAICodexResponsesGateway) providerRequest(request Request, llmCallID string, payload map[string]any) (map[string]any, int, StreamLlmRequest, error) {
	debugPayload, redactedHints := sanitizeDebugPayloadJSON(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, StreamLlmRequest{}, err
	}
	baseURL := g.transport.cfg.BaseURL
	path := openAICodexResponsesPath
	stats := ComputeRequestStats(request)
	networkAttempted := false
	return payload, len(encoded), StreamLlmRequest{LlmCallID: llmCallID, ProviderKind: openAICodexResponsesProviderKind, APIMode: openAICodexResponsesAPIMode, BaseURL: &baseURL, Path: &path, InputJSON: request.ToJSON(), PayloadJSON: debugPayload, RedactedHints: redactedHints, SystemBytes: stats.SystemBytes, ToolsBytes: stats.ToolsBytes, MessagesBytes: stats.MessagesBytes, AbstractRequestBytes: stats.AbstractRequestBytes, ProviderPayloadBytes: len(encoded), ImagePartCount: stats.ImagePartCount, Base64ImageBytes: stats.Base64ImageBytes, NetworkAttempted: &networkAttempted, RoleBytes: stats.RoleBytes, ToolSchemaBytesMap: stats.ToolSchemaBytesMap, StablePrefixHash: stats.StablePrefixHash, SessionPrefixHash: stats.SessionPrefixHash, VolatileTailHash: stats.VolatileTailHash, ToolSchemaHash: stats.ToolSchemaHash, StablePrefixBytes: stats.StablePrefixBytes, SessionPrefixBytes: stats.SessionPrefixBytes, VolatileTailBytes: stats.VolatileTailBytes, CacheCandidateBytes: stats.CacheCandidateBytes}, nil
}

func (g *openAICodexResponsesGateway) codexAuth() (string, string, *GatewayError) {
	token := strings.TrimSpace(g.transport.cfg.APIKey)
	if token == "" {
		return "", "", &GatewayError{ErrorClass: ErrorClassConfigMissing, Message: "Codex OAuth token is required"}
	}
	accountID := openAICodexHeaderValue(g.transport.cfg.DefaultHeaders, "chatgpt-account-id")
	if accountID == "" {
		accountID = extractOpenAICodexAccountID(token)
	}
	if accountID == "" {
		return "", "", &GatewayError{ErrorClass: ErrorClassConfigMissing, Message: "Codex chatgpt account id is required"}
	}
	return token, accountID, nil
}

func (g *openAICodexResponsesGateway) applyHeaders(req *http.Request, token string, accountID string) {
	g.transport.applyDefaultHeaders(req)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "pi")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", openAICodexUserAgent())
}

func (g *openAICodexResponsesGateway) emitDebugChunk(llmCallID string, raw string, statusCode *int, yield func(StreamEvent) error) error {
	if !g.transport.cfg.EmitDebugEvents || strings.TrimSpace(raw) == "" {
		return nil
	}
	truncatedRaw, rawTruncated := truncateUTF8(raw, openAIMaxDebugChunkBytes)
	var chunkJSON any
	_ = json.Unmarshal([]byte(raw), &chunkJSON)
	return yield(StreamLlmResponseChunk{LlmCallID: llmCallID, ProviderKind: openAICodexResponsesProviderKind, APIMode: openAICodexResponsesAPIMode, Raw: truncatedRaw, ChunkJSON: chunkJSON, StatusCode: statusCode, Truncated: rawTruncated})
}

func openAICodexHTTPError(resp *http.Response, body []byte, payloadBytes int, responseCapture *providerResponseCapture) GatewayError {
	message, details := openAIErrorMessageAndDetails(body, resp.StatusCode, "Codex responses request failed")
	details["provider_kind"] = openAICodexResponsesProviderKind
	details["api_mode"] = openAICodexResponsesAPIMode
	details["network_attempted"] = true
	details["streaming"] = true
	if requestID := sdkProviderRequestID(resp.Header); requestID != "" {
		details["provider_request_id"] = requestID
	}
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		details = OversizeFailureDetails(payloadBytes, OversizePhaseProvider, details)
	}
	details = mergeProviderResponseCaptureDetails(details, responseCapture)
	return GatewayError{ErrorClass: classifyOpenAIStatus(resp.StatusCode, details), Message: message, Details: details}
}

func openAICodexStreamErrorToGateway(err error, payloadBytes int, responseCapture *providerResponseCapture, ctx context.Context) GatewayError {
	details := sdkTransportErrorDetails(err, openAICodexResponsesProviderKind, openAICodexResponsesAPIMode, true, true)
	details = mergeContextErrorDetails(details, err, ctx)
	details = mergeProviderResponseCaptureDetails(details, responseCapture)
	if capturedStatus, _ := details["status_code"].(int); capturedStatus == http.StatusRequestEntityTooLarge {
		details = OversizeFailureDetails(payloadBytes, OversizePhaseProvider, details)
	}
	return GatewayError{ErrorClass: ErrorClassProviderRetryable, Message: "Codex responses network error", Details: details}
}

func normalizeOpenAICodexResponsesBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return defaultOpenAICodexResponsesBaseURL
	}
	if strings.HasSuffix(trimmed, "/codex/responses") {
		return strings.TrimSuffix(trimmed, "/responses")
	}
	if strings.HasSuffix(trimmed, "/backend-api") {
		return trimmed + "/codex"
	}
	return trimmed
}

func applyOpenAICodexResponsesPayloadDefaults(payload map[string]any) {
	payload["stream"] = true
	payload["store"] = false
	ensureOpenAICodexTextVerbosity(payload)
	ensureOpenAICodexInclude(payload, "reasoning.encrypted_content")
	if _, exists := payload["tool_choice"]; !exists {
		payload["tool_choice"] = "auto"
	}
	if _, exists := payload["parallel_tool_calls"]; !exists {
		payload["parallel_tool_calls"] = true
	}
	applyOpenAICodexToolDefaults(payload["tools"])
}

func ensureOpenAICodexTextVerbosity(payload map[string]any) {
	text, ok := payload["text"].(map[string]any)
	if !ok || text == nil {
		payload["text"] = map[string]any{"verbosity": "medium"}
		return
	}
	if _, exists := text["verbosity"]; !exists {
		text["verbosity"] = "medium"
	}
}

func ensureOpenAICodexInclude(payload map[string]any, value string) {
	switch include := payload["include"].(type) {
	case []string:
		for _, item := range include {
			if item == value {
				return
			}
		}
		payload["include"] = append(include, value)
	case []any:
		for _, item := range include {
			if text, _ := item.(string); text == value {
				return
			}
		}
		payload["include"] = append(include, value)
	default:
		payload["include"] = []any{value}
	}
}

func applyOpenAICodexToolDefaults(raw any) {
	switch tools := raw.(type) {
	case []map[string]any:
		for _, tool := range tools {
			if _, exists := tool["strict"]; !exists {
				tool["strict"] = nil
			}
		}
	case []any:
		for _, rawTool := range tools {
			tool, _ := rawTool.(map[string]any)
			if tool == nil {
				continue
			}
			if _, exists := tool["strict"]; !exists {
				tool["strict"] = nil
			}
		}
	}
}

func normalizeOpenAICodexSSEData(data string) (string, bool) {
	raw := strings.TrimSpace(data)
	if raw == "" || raw == "[DONE]" {
		return "", false
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return raw, true
	}
	typ, _ := root["type"].(string)
	if typ != "response.done" && typ != "response.completed" {
		return raw, true
	}
	root["type"] = "response.completed"
	if response, _ := root["response"].(map[string]any); response != nil {
		if status := normalizeOpenAICodexStatus(response["status"]); status != "" {
			response["status"] = status
		} else {
			delete(response, "status")
		}
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return raw, true
	}
	return string(encoded), true
}

func normalizeOpenAICodexStatus(status any) string {
	text, _ := status.(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if _, ok := openAICodexResponseStatuses[text]; ok {
		return text
	}
	return ""
}

func openAICodexHeaderValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractOpenAICodexAccountID(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	auth, _ := claims[openAICodexAuthClaimPath].(map[string]any)
	accountID, _ := auth["chatgpt_account_id"].(string)
	return strings.TrimSpace(accountID)
}

func openAICodexUserAgent() string {
	return fmt.Sprintf("pi (%s %s)", runtime.GOOS, runtime.GOARCH)
}
