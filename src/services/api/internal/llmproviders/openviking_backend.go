package llmproviders

import "strings"

const (
	OpenVikingBackendOpenAI           = "openai"
	OpenVikingBackendAzure            = "azure"
	OpenVikingBackendVolcengine       = "volcengine"
	OpenVikingBackendOpenAICompatible = "openai_compatible"
	openVikingBackendLegacyLiteLLM    = "litellm"
)

var validOpenVikingBackends = map[string]struct{}{
	OpenVikingBackendOpenAI:           {},
	OpenVikingBackendAzure:            {},
	OpenVikingBackendVolcengine:       {},
	OpenVikingBackendOpenAICompatible: {},
	openVikingBackendLegacyLiteLLM:    {},
}

func IsValidOpenVikingBackend(raw string) bool {
	_, ok := validOpenVikingBackends[normalizeOpenVikingBackend(raw)]
	return ok
}

func ResolveOpenVikingBackend(provider string, advancedJSON map[string]any) string {
	if backend := OpenVikingBackendFromAdvancedJSON(advancedJSON); IsValidOpenVikingBackend(backend) {
		return normalizeOpenVikingBackend(backend)
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return OpenVikingBackendOpenAI
	case "anthropic", "gemini", "deepseek", "doubao", "qwen", "yuanbao", "kimi", "zenmax":
		return OpenVikingBackendOpenAICompatible
	default:
		return ""
	}
}

func OpenVikingRenderedProvider(raw string) string {
	switch normalizeOpenVikingBackend(raw) {
	case OpenVikingBackendOpenAICompatible:
		return openVikingBackendLegacyLiteLLM
	default:
		return normalizeOpenVikingBackend(raw)
	}
}

func IsSupportedOpenVikingEmbeddingBackend(raw string) bool {
	switch normalizeOpenVikingBackend(raw) {
	case OpenVikingBackendOpenAI, OpenVikingBackendVolcengine:
		return true
	default:
		return false
	}
}

func normalizeOpenVikingBackend(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case openVikingBackendLegacyLiteLLM, OpenVikingBackendOpenAICompatible:
		return OpenVikingBackendOpenAICompatible
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}
