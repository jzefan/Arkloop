package pipeline

import (
	"errors"
	"testing"

	"arkloop/services/shared/localproviders"
	"arkloop/services/worker/internal/llm"
)

func TestLocalProviderResolveGatewayErrorUsageLimit(t *testing.T) {
	gatewayErr := localProviderResolveGatewayError(localproviders.ClaudeCodeProviderID, &localproviders.OAuthHTTPError{
		StatusCode: 429,
		Message:    "Usage limit reached",
		Body:       `{"error":{"message":"Usage limit reached"}}`,
	})
	if gatewayErr.ErrorClass != llm.ErrorClassProviderUsageLimit {
		t.Fatalf("ErrorClass = %q, want %q", gatewayErr.ErrorClass, llm.ErrorClassProviderUsageLimit)
	}
	if gatewayErr.Message != "Claude Code usage limit reached" {
		t.Fatalf("Message = %q", gatewayErr.Message)
	}
}

func TestLocalProviderResolveGatewayErrorMissingConfig(t *testing.T) {
	gatewayErr := localProviderResolveGatewayError(localproviders.ClaudeCodeProviderID, localproviders.ErrCredentialUnavailable)
	if gatewayErr.ErrorClass != llm.ErrorClassConfigMissing {
		t.Fatalf("ErrorClass = %q, want %q", gatewayErr.ErrorClass, llm.ErrorClassConfigMissing)
	}
}

func TestLocalProviderResolveGatewayErrorRefreshFailure(t *testing.T) {
	gatewayErr := localProviderResolveGatewayError(localproviders.ClaudeCodeProviderID, errors.New("oauth refresh failed"))
	if gatewayErr.ErrorClass != llm.ErrorClassConfigInvalid {
		t.Fatalf("ErrorClass = %q, want %q", gatewayErr.ErrorClass, llm.ErrorClassConfigInvalid)
	}
}
