package llmproviders

import (
	"errors"
	"strings"
)

const (
	anthropicAdvancedVersionKey      = "anthropic_version"
	anthropicAdvancedExtraHeadersKey = "extra_headers"
	anthropicBetaHeaderName          = "anthropic-beta"
)

func ValidateAdvancedJSONForProvider(provider string, advancedJSON map[string]any) error {
	if strings.TrimSpace(provider) != "anthropic" || advancedJSON == nil {
		return nil
	}
	return validateAnthropicAdvancedJSON(advancedJSON)
}

func validateAnthropicAdvancedJSON(advancedJSON map[string]any) error {
	if advancedJSON == nil {
		return nil
	}
	if rawVersion, ok := advancedJSON[anthropicAdvancedVersionKey]; ok {
		version, ok := rawVersion.(string)
		if !ok || strings.TrimSpace(version) == "" {
			return errors.New("advanced_json.anthropic_version must be a non-empty string")
		}
	}

	rawHeaders, ok := advancedJSON[anthropicAdvancedExtraHeadersKey]
	if !ok {
		return nil
	}
	headers, ok := rawHeaders.(map[string]any)
	if !ok {
		return errors.New("advanced_json.extra_headers must be an object")
	}
	for key, value := range headers {
		headerName := strings.ToLower(strings.TrimSpace(key))
		if headerName != anthropicBetaHeaderName {
			return errors.New("advanced_json.extra_headers only supports anthropic-beta")
		}
		headerValue, ok := value.(string)
		if !ok || strings.TrimSpace(headerValue) == "" {
			return errors.New("advanced_json.extra_headers.anthropic-beta must be a non-empty string")
		}
	}
	return nil
}
