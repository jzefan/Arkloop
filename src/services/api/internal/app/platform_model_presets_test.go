//go:build !desktop

package app

import "testing"

func TestPlatformModelPresetSpecsFromEnvSkipsMissingKeys(t *testing.T) {
	env := map[string]string{
		"ARKLOOP_DEEPSEEK_API_KEY": " deepseek-key ",
		"ARKLOOP_QWEN_API_KEY":     "",
		"ARKLOOP_DOUBAO_API_KEY":   "   ",
	}

	specs := platformModelPresetSpecsFromEnv(func(key string) string { return env[key] })
	if len(specs) != 1 {
		t.Fatalf("expected only deepseek preset, got %#v", specs)
	}
	if specs[0].Provider != "deepseek" || specs[0].APIKey != "deepseek-key" {
		t.Fatalf("unexpected preset: %#v", specs[0])
	}
}

func TestPlatformModelPresetSpecsFromEnvAllowsModelOverride(t *testing.T) {
	env := map[string]string{
		"ARKLOOP_QWEN_API_KEY": "qwen-key",
		"ARKLOOP_QWEN_MODELS":  " qwen-plus ,, qwen-max ",
	}

	specs := platformModelPresetSpecsFromEnv(func(key string) string { return env[key] })
	if len(specs) != 1 {
		t.Fatalf("expected qwen preset, got %#v", specs)
	}
	want := []string{"qwen-plus", "qwen-max"}
	if len(specs[0].Models) != len(want) {
		t.Fatalf("unexpected models: %#v", specs[0].Models)
	}
	for i := range want {
		if specs[0].Models[i] != want[i] {
			t.Fatalf("unexpected models: got %#v want %#v", specs[0].Models, want)
		}
	}
}

func TestPlatformModelPresetSpecsFromEnvAcceptsCommonKeyAliases(t *testing.T) {
	env := map[string]string{
		"DEEPSEEK_API_KEY":  "deepseek-alias",
		"DASHSCOPE_API_KEY": "qwen-alias",
		"ARK_API_KEY":       "doubao-alias",
	}

	specs := platformModelPresetSpecsFromEnv(func(key string) string { return env[key] })
	if len(specs) != 3 {
		t.Fatalf("expected three presets, got %#v", specs)
	}
	want := map[string]string{
		"deepseek": "deepseek-alias",
		"qwen":     "qwen-alias",
		"doubao":   "doubao-alias",
	}
	for _, spec := range specs {
		if spec.APIKey != want[spec.Provider] {
			t.Fatalf("unexpected key for %s: got %q want %q", spec.Provider, spec.APIKey, want[spec.Provider])
		}
	}
}

// TestPlatformModelPresetDefaultsAreRealIDs locks in the requirement that the
// hardcoded default model IDs match real upstream model identifiers. Earlier
// defaults like "Qwen3.6-27B" and "Doubao-Seed-2.0-Mini" were placeholders
// that DashScope / Volcengine Ark rejected with "model does not exist", which
// surfaced as Lua errors in industry-education-evaluator/agent.lua.
//
// DeepSeek model IDs follow DeepSeek's own naming (deepseek-v4-* is the
// current generation; deepseek-chat / deepseek-reasoner are legacy aliases
// scheduled for deprecation per https://api-docs.deepseek.com/).
func TestPlatformModelPresetDefaultsAreRealIDs(t *testing.T) {
	env := map[string]string{
		"DEEPSEEK_API_KEY":  "deepseek-key",
		"DASHSCOPE_API_KEY": "qwen-key",
		"ARK_API_KEY":       "doubao-key",
	}

	specs := platformModelPresetSpecsFromEnv(func(key string) string { return env[key] })

	want := map[string][]string{
		// DeepSeek's current-generation model IDs per its public docs.
		"deepseek": {"deepseek-v4-flash", "deepseek-v4-pro"},
		// DashScope OpenAI-compatible mode exposes the Qwen3 generation IDs.
		"qwen": {"qwen3.5-plus", "qwen3-max-2026-01-23"},
		// Volcengine Ark requires either a dated foundation model ID or a user
		// endpoint ID. Operators typically override via ARKLOOP_DOUBAO_MODELS;
		// the default must still be a syntactically real Ark model ID.
		"doubao": {"doubao-seed-2-0-lite-260428", "doubao-seed-2-0-mini-260428"},
	}
	got := map[string][]string{}
	for _, spec := range specs {
		got[spec.Provider] = spec.Models
	}
	for provider, expected := range want {
		actual, ok := got[provider]
		if !ok {
			t.Fatalf("missing preset for %s", provider)
		}
		if len(actual) != len(expected) {
			t.Fatalf("%s default models: got %#v want %#v", provider, actual, expected)
		}
		for i := range expected {
			if actual[i] != expected[i] {
				t.Fatalf("%s default model[%d]: got %q want %q", provider, i, actual[i], expected[i])
			}
		}
	}

	// Forbid resurrecting the historical placeholder names that caused the
	// "model does not exist" error in双高产教融合评估.
	forbidden := map[string]struct{}{
		"Qwen3.5-Plus":         {},
		"Qwen3.6-27B":          {},
		"Doubao-Seed-2.0-Lite": {},
		"Doubao-Seed-2.0-Mini": {},
	}
	for _, spec := range specs {
		for _, model := range spec.Models {
			if _, bad := forbidden[model]; bad {
				t.Fatalf("preset %s reintroduced placeholder model id %q", spec.Provider, model)
			}
		}
	}
}
