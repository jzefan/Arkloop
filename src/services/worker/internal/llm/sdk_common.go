package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

func sdkHTTPClient(t protocolTransport) *http.Client {
	return t.client
}

func sdkBaseURL(t protocolTransport) string {
	return t.cfg.BaseURL
}

func classifyHTTPStatus(status int) string {
	switch status {
	case 408, 425, 429:
		return ErrorClassProviderRetryable
	default:
		if status >= 500 && status <= 599 {
			return ErrorClassProviderRetryable
		}
		return ErrorClassProviderNonRetryable
	}
}

func errorClassFromStatus(status int) string {
	return classifyHTTPStatus(status)
}

func sdkTransportErrorDetails(err error, providerKind string, apiMode string, streaming bool, networkAttempted bool) map[string]any {
	details := map[string]any{
		"reason":            err.Error(),
		"error_type":        fmt.Sprintf("%T", err),
		"provider_kind":     providerKind,
		"network_attempted": networkAttempted,
	}
	if strings.TrimSpace(apiMode) != "" {
		details["api_mode"] = strings.TrimSpace(apiMode)
	}
	if streaming {
		details["streaming"] = true
	}
	if sdkJSONEOFError(err) {
		details["provider_response_parse_error"] = true
	}
	return details
}

func mergeContextErrorDetails(details map[string]any, err error, ctx context.Context) map[string]any {
	if err == nil && ctx == nil {
		return details
	}
	if details == nil {
		details = map[string]any{}
	}
	if errors.Is(err, context.Canceled) {
		details["context_canceled"] = true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		details["context_deadline_exceeded"] = true
	}
	if errors.Is(err, errStreamIdleTimeout) {
		details["stream_idle_timeout"] = true
	}
	if ctx == nil {
		return details
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		details["context_done"] = true
		details["context_error"] = ctxErr.Error()
		if errors.Is(ctxErr, context.Canceled) {
			details["context_canceled"] = true
		}
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			details["context_deadline_exceeded"] = true
		}
	}
	if cause := context.Cause(ctx); cause != nil {
		details["context_cause"] = cause.Error()
		if errors.Is(cause, errStreamIdleTimeout) {
			details["stream_idle_timeout"] = true
		}
	}
	return details
}

func sdkJSONEOFError(err error) bool {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) && strings.Contains(syntaxErr.Error(), "unexpected end of JSON input") {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	reason := strings.ToLower(err.Error())
	return strings.Contains(reason, "unexpected end of json input") || strings.Contains(reason, "unexpected eof")
}

func sdkProviderRequestID(headers http.Header) string {
	for _, key := range []string{"x-request-id", "request-id", "anthropic-request-id", "openai-request-id"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func costFromFloat64(value *float64) *Cost {
	if value == nil || *value <= 0 {
		return nil
	}
	return &Cost{
		Currency:     "USD",
		AmountMicros: int(math.Round(*value * 1_000_000)),
	}
}
