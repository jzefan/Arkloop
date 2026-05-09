package llmproviders

import (
	"testing"

	"arkloop/services/api/internal/data"
)

func TestResolveCatalogProtocolConfigOpenAI(t *testing.T) {
	mode := "responses"
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider:      "openai",
		OpenAIAPIMode: &mode,
	}, "sk-test")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindOpenAIResponses {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindOpenAIResponses)
	}
	if cfg.BaseURL != defaultOpenAIBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultOpenAIBaseURL)
	}
}

func TestResolveCatalogProtocolConfigOpenAIRejectsInvalidMode(t *testing.T) {
	mode := "bad-mode"
	_, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider:      "openai",
		OpenAIAPIMode: &mode,
	}, "sk-test")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "invalid openai_api_mode: bad-mode" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveCatalogProtocolConfigDeepSeekDefaultsToChatCompletions(t *testing.T) {
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "deepseek",
	}, "sk-deepseek")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindOpenAIChatCompletions {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindOpenAIChatCompletions)
	}
	if cfg.BaseURL != defaultDeepSeekBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultDeepSeekBaseURL)
	}
	if cfg.OpenAI.APIMode != "chat_completions" {
		t.Fatalf("OpenAI.APIMode = %q, want chat_completions", cfg.OpenAI.APIMode)
	}
}

func TestResolveCatalogProtocolConfigDoubaoDefaultsToChatCompletions(t *testing.T) {
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "doubao",
	}, "sk-doubao")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindOpenAIChatCompletions {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindOpenAIChatCompletions)
	}
	if cfg.BaseURL != defaultDoubaoBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultDoubaoBaseURL)
	}
	if cfg.OpenAI.APIMode != "chat_completions" {
		t.Fatalf("OpenAI.APIMode = %q, want chat_completions", cfg.OpenAI.APIMode)
	}
}

func TestResolveCatalogProtocolConfigQwenDefaultsToChatCompletions(t *testing.T) {
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "qwen",
	}, "sk-qwen")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindOpenAIChatCompletions {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindOpenAIChatCompletions)
	}
	if cfg.BaseURL != defaultQwenBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultQwenBaseURL)
	}
	if cfg.OpenAI.APIMode != "chat_completions" {
		t.Fatalf("OpenAI.APIMode = %q, want chat_completions", cfg.OpenAI.APIMode)
	}
}

func TestResolveCatalogProtocolConfigYuanbaoDefaultsToChatCompletions(t *testing.T) {
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "yuanbao",
	}, "sk-yuanbao")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindOpenAIChatCompletions {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindOpenAIChatCompletions)
	}
	if cfg.BaseURL != defaultYuanbaoBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultYuanbaoBaseURL)
	}
	if cfg.OpenAI.APIMode != "chat_completions" {
		t.Fatalf("OpenAI.APIMode = %q, want chat_completions", cfg.OpenAI.APIMode)
	}
}

func TestResolveCatalogProtocolConfigKimiDefaultsToChatCompletions(t *testing.T) {
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "kimi",
	}, "sk-kimi")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindOpenAIChatCompletions {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindOpenAIChatCompletions)
	}
	if cfg.BaseURL != defaultKimiBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultKimiBaseURL)
	}
	if cfg.OpenAI.APIMode != "chat_completions" {
		t.Fatalf("OpenAI.APIMode = %q, want chat_completions", cfg.OpenAI.APIMode)
	}
}

func TestResolveCatalogProtocolConfigZenMaxProtocols(t *testing.T) {
	tests := []struct {
		name          string
		protocol      string
		wantKind      ProtocolKind
		wantBaseURL   string
		wantAPIMode   string
		wantGemini    bool
		wantAnthropic bool
	}{
		{
			name:        "openai",
			protocol:    "openai",
			wantKind:    ProtocolKindOpenAIChatCompletions,
			wantBaseURL: defaultZenMaxOpenAIBaseURL,
			wantAPIMode: "chat_completions",
		},
		{
			name:          "claude",
			protocol:      "anthropic",
			wantKind:      ProtocolKindAnthropicMessages,
			wantBaseURL:   defaultZenMaxAnthropicBaseURL,
			wantAnthropic: true,
		},
		{
			name:        "gemini",
			protocol:    "gemini",
			wantKind:    ProtocolKindGeminiGenerateContent,
			wantBaseURL: defaultZenMaxGeminiCatalogBaseURL,
			wantGemini:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
				Provider: "zenmax",
				AdvancedJSON: map[string]any{
					"zenmax_protocol": tt.protocol,
				},
			}, "sk-zenmax")
			if err != nil {
				t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
			}
			if cfg.Kind != tt.wantKind {
				t.Fatalf("Kind = %q, want %q", cfg.Kind, tt.wantKind)
			}
			if cfg.BaseURL != tt.wantBaseURL {
				t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, tt.wantBaseURL)
			}
			if tt.wantAPIMode != "" && cfg.OpenAI.APIMode != tt.wantAPIMode {
				t.Fatalf("OpenAI.APIMode = %q, want %q", cfg.OpenAI.APIMode, tt.wantAPIMode)
			}
			if tt.wantAnthropic && cfg.Anthropic.Version == "" {
				t.Fatal("expected anthropic protocol config")
			}
			if tt.wantGemini && cfg.Gemini != (GeminiCatalogConfig{}) {
				t.Fatalf("Gemini config = %#v, want empty config", cfg.Gemini)
			}
		})
	}
}

func TestResolveCatalogProtocolConfigAnthropicDefaultsToHostBase(t *testing.T) {
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "anthropic",
	}, "sk-ant")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.Kind != ProtocolKindAnthropicMessages {
		t.Fatalf("Kind = %q, want %q", cfg.Kind, ProtocolKindAnthropicMessages)
	}
	if cfg.BaseURL != defaultAnthropicBaseURL {
		t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, defaultAnthropicBaseURL)
	}
	if anthropicCatalogPath(cfg.BaseURL) != "/v1/models" {
		t.Fatalf("anthropicCatalogPath(%q) = %q, want /v1/models", cfg.BaseURL, anthropicCatalogPath(cfg.BaseURL))
	}
}

func TestResolveCatalogProtocolConfigAnthropicNormalizesMiniMaxRoot(t *testing.T) {
	baseURL := "https://api.minimaxi.com/anthropic"
	cfg, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "anthropic",
		BaseURL:  &baseURL,
	}, "sk-ant")
	if err != nil {
		t.Fatalf("ResolveCatalogProtocolConfig() error = %v", err)
	}
	if cfg.BaseURL != "https://api.minimaxi.com/anthropic/v1" {
		t.Fatalf("BaseURL = %q, want minimax v1 base", cfg.BaseURL)
	}
	if anthropicCatalogPath(cfg.BaseURL) != "/models" {
		t.Fatalf("anthropicCatalogPath(%q) = %q, want /models", cfg.BaseURL, anthropicCatalogPath(cfg.BaseURL))
	}
}

func TestResolveCatalogProtocolConfigAnthropicRejectsInvalidExtraHeaders(t *testing.T) {
	_, err := ResolveCatalogProtocolConfig(data.LlmCredential{
		Provider: "anthropic",
		AdvancedJSON: map[string]any{
			"extra_headers": map[string]any{
				"x-custom": "bad",
			},
		},
	}, "sk-ant")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "advanced_json.extra_headers only supports anthropic-beta" {
		t.Fatalf("unexpected error: %v", err)
	}
}
