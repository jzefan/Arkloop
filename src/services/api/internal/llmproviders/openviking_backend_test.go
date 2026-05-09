package llmproviders

import "testing"

func TestResolveOpenVikingBackendOpenAICompatibleProviders(t *testing.T) {
	for _, provider := range []string{"deepseek", "doubao", "qwen", "yuanbao", "kimi", "zenmax"} {
		t.Run(provider, func(t *testing.T) {
			got := ResolveOpenVikingBackend(provider, nil)
			if got != OpenVikingBackendOpenAICompatible {
				t.Fatalf("ResolveOpenVikingBackend(%q) = %q, want %q", provider, got, OpenVikingBackendOpenAICompatible)
			}
		})
	}
}
