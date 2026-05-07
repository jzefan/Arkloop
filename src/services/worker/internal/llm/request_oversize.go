package llm

import "net/http"

const RequestPayloadLimitBytes = 5 * 1024 * 1024

const (
	OversizePhasePreflight = "preflight"
	OversizePhaseProvider  = "provider"
)

func RequestPayloadTooLarge(payloadBytes int) bool {
	return payloadBytes > RequestPayloadLimitBytes
}

func RequestExceedsLimits(payloadBytes int, estimatedTokens int, contextWindowTokens int) bool {
	return RequestPayloadTooLarge(payloadBytes) || (contextWindowTokens > 0 && estimatedTokens > contextWindowTokens)
}

func OversizeFailureDetails(payloadBytes int, phase string, base map[string]any) map[string]any {
	details := map[string]any{}
	for key, value := range base {
		details[key] = value
	}
	details["status_code"] = http.StatusRequestEntityTooLarge
	details["oversize_phase"] = phase
	details["payload_bytes"] = payloadBytes
	details["payload_limit_bytes"] = RequestPayloadLimitBytes
	return details
}

func PreflightOversizeFailure(llmCallID string, payloadBytes int) StreamRunFailed {
	return StreamRunFailed{
		LlmCallID: llmCallID,
		Error: GatewayError{
			ErrorClass: ErrorClassProviderNonRetryable,
			Message:    "request payload too large",
			Details:    OversizeFailureDetails(payloadBytes, OversizePhasePreflight, map[string]any{"network_attempted": false}),
		},
	}
}
