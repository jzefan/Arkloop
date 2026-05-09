package pipeline_test

import (
	"context"
	"os"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
)

type auxGateway struct{}

func (s auxGateway) Stream(_ context.Context, _ llm.Request, _ func(llm.StreamEvent) error) error {
	return nil
}

func buildStubRouterConfig() routing.ProviderRoutingConfig {
	return routing.ProviderRoutingConfig{
		DefaultRouteID: "route-default",
		Credentials: []routing.ProviderCredential{
			{ID: "cred-stub", Name: "stub-cred", OwnerKind: routing.CredentialScopePlatform, ProviderKind: routing.ProviderKindStub},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-default", Model: "stub-model", CredentialID: "cred-stub", Multiplier: 1.0},
		},
	}
}

func TestRoutingMiddleware_AuxGatewaySelected(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)
	stub := auxGateway{}

	mw := pipeline.NewRoutingMiddleware(
		router, nil, stub, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	var reached bool
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		reached = true
		if rc.Gateway == nil {
			t.Fatal("Gateway 未设置")
		}
		if rc.SelectedRoute == nil {
			t.Fatal("SelectedRoute 未设置")
		}
		if rc.SelectedRoute.Route.ID != "route-default" {
			t.Fatalf("路由 ID = %q, 期望 route-default", rc.SelectedRoute.Route.ID)
		}
		if rc.ResolveGatewayForRouteID == nil {
			t.Fatal("ResolveGatewayForRouteID 未设置")
		}
		if rc.ResolveGatewayForAgentName == nil {
			t.Fatal("ResolveGatewayForAgentName 未设置")
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reached {
		t.Fatal("终端 handler 未执行")
	}
}

func TestRoutingMiddleware_NilDbPoolUsesStaticRouter(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)
	stub := auxGateway{}

	mw := pipeline.NewRoutingMiddleware(
		router, nil, stub, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	var gatewaySet bool
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gatewaySet = rc.Gateway != nil
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gatewaySet {
		t.Fatal("dbPool 为 nil 时应使用 static router 选中路由")
	}
}

func TestRoutingMiddleware_EmptyRouterNoSelectedRoute(t *testing.T) {
	emptyCfg := routing.ProviderRoutingConfig{}
	router := routing.NewProviderRouter(emptyCfg)
	stub := auxGateway{}

	mw := pipeline.NewRoutingMiddleware(
		router, nil, stub, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	// 空路由配置 -> selected=nil -> 尝试 appendAndCommitSingle(nil pool) -> panic
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("空路由配置下应 panic（nil pool 调 BeginTx）")
		}
	}()

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, _ *pipeline.RunContext) error {
		t.Fatal("不应到达终端 handler")
		return nil
	})
	_ = h(context.Background(), rc)
}

func TestRoutingMiddleware_UnknownProviderKind(t *testing.T) {
	cfg := routing.ProviderRoutingConfig{
		DefaultRouteID: "route-unknown",
		Credentials: []routing.ProviderCredential{
			{ID: "cred-x", Name: "unknown-cred", OwnerKind: routing.CredentialScopePlatform, ProviderKind: "unknown_kind"},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-unknown", Model: "x-model", CredentialID: "cred-x", Multiplier: 1.0},
		},
	}
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	// unknown provider_kind -> gatewayFromCredential 返回 error -> 尝试 appendAndCommitSingle(nil pool) -> panic
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("未知 provider_kind 应 panic（nil pool 调 BeginTx）")
		}
	}()

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, _ *pipeline.RunContext) error {
		t.Fatal("不应到达终端 handler")
		return nil
	})
	_ = h(context.Background(), rc)
}

func TestRoutingMiddleware_ResolveGatewayForRouteID_EmptyFallbackCurrent(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)
	stub := auxGateway{}

	mw := pipeline.NewRoutingMiddleware(
		router, nil, stub, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		// 空 route_id 应回退当前路由
		gw, sel, err := rc.ResolveGatewayForRouteID(context.Background(), "")
		if err != nil {
			t.Fatalf("空 route_id 应回退当前路由, 但返回 error: %v", err)
		}
		if gw == nil || sel == nil {
			t.Fatal("空 route_id 应返回当前 gateway 和 route")
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoutingMiddleware_ResolveGatewayForRouteID_ValidRoute(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)
	stub := auxGateway{}

	mw := pipeline.NewRoutingMiddleware(
		router, nil, stub, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gw, sel, err := rc.ResolveGatewayForRouteID(context.Background(), "route-default")
		if err != nil {
			t.Fatalf("合法 route_id 不应返回 error: %v", err)
		}
		if gw == nil {
			t.Fatal("gateway 不应为 nil")
		}
		if sel.Route.ID != "route-default" {
			t.Fatalf("route ID = %q, 期望 route-default", sel.Route.ID)
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoutingMiddleware_ResolveGatewayForRouteID_NotFound(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)
	stub := auxGateway{}

	mw := pipeline.NewRoutingMiddleware(
		router, nil, stub, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		_, _, err := rc.ResolveGatewayForRouteID(context.Background(), "nonexistent-route")
		if err == nil {
			t.Fatal("不存在的 route_id 应返回 error")
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoutingMiddleware_OpenAIGateway_WithEnvApiKey(t *testing.T) {
	const envKey = "ARKLOOP_TEST_OPENAI_KEY"
	if err := os.Setenv(envKey, "sk-test-12345"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer func() { _ = os.Unsetenv(envKey) }()

	cfg := routing.ProviderRoutingConfig{
		DefaultRouteID: "route-openai",
		Credentials: []routing.ProviderCredential{
			{
				ID: "cred-openai", Name: "openai-cred",
				OwnerKind: routing.CredentialScopePlatform, ProviderKind: routing.ProviderKindOpenAI,
				APIKeyEnv: strPtr(envKey),
			},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-openai", Model: "gpt-4", CredentialID: "cred-openai", Multiplier: 1.0},
		},
	}
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	var gatewayOK bool
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gatewayOK = rc.Gateway != nil
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gatewayOK {
		t.Fatal("OpenAI gateway 应被成功创建")
	}
}

func TestRoutingMiddleware_AnthropicGateway_WithDirectApiKey(t *testing.T) {
	cfg := routing.ProviderRoutingConfig{
		DefaultRouteID: "route-anthropic",
		Credentials: []routing.ProviderCredential{
			{
				ID: "cred-anthropic", Name: "anthropic-cred",
				OwnerKind: routing.CredentialScopePlatform, ProviderKind: routing.ProviderKindAnthropic,
				APIKeyValue: strPtr("sk-ant-test-key"),
			},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-anthropic", Model: "claude-3", CredentialID: "cred-anthropic", Multiplier: 1.0},
		},
	}
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	var gatewayOK bool
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gatewayOK = rc.Gateway != nil
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gatewayOK {
		t.Fatal("Anthropic gateway 应被成功创建")
	}
}

func TestResolveGatewayConfigFromSelectedRoute_ZenMaxProtocols(t *testing.T) {
	tests := []struct {
		name        string
		protocol    string
		wantKind    llm.ProtocolKind
		wantBaseURL string
	}{
		{name: "openai", protocol: "openai", wantKind: llm.ProtocolKindOpenAIChatCompletions, wantBaseURL: "https://zenmux.ai/api/v1"},
		{name: "claude", protocol: "anthropic", wantKind: llm.ProtocolKindAnthropicMessages, wantBaseURL: "https://zenmux.ai/api/anthropic"},
		{name: "gemini", protocol: "gemini", wantKind: llm.ProtocolKindGeminiGenerateContent, wantBaseURL: "https://zenmux.ai/api/vertex-ai"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := pipeline.ResolveGatewayConfigFromSelectedRoute(routing.SelectedProviderRoute{
				Route: routing.ProviderRouteRule{ID: "route", Model: "model"},
				Credential: routing.ProviderCredential{
					ID:           "cred",
					ProviderKind: routing.ProviderKindZenMax,
					APIKeyValue:  strPtr("sk-zenmax"),
					AdvancedJSON: map[string]any{
						"zenmax_protocol": tt.protocol,
					},
				},
			}, false, 0)
			if err != nil {
				t.Fatalf("ResolveGatewayConfigFromSelectedRoute() error = %v", err)
			}
			if cfg.ProtocolKind != tt.wantKind {
				t.Fatalf("ProtocolKind = %q, want %q", cfg.ProtocolKind, tt.wantKind)
			}
			if cfg.Transport.BaseURL != tt.wantBaseURL {
				t.Fatalf("BaseURL = %q, want %q", cfg.Transport.BaseURL, tt.wantBaseURL)
			}
		})
	}
}

func TestResolveGatewayConfigFromSelectedRoute_OpenAICompatibleVendorDefaults(t *testing.T) {
	tests := []struct {
		name        string
		kind        routing.ProviderKind
		model       string
		wantBaseURL string
	}{
		{name: "deepseek", kind: routing.ProviderKindDeepSeek, model: "deepseek-chat", wantBaseURL: "https://api.deepseek.com"},
		{name: "doubao", kind: routing.ProviderKindDoubao, model: "doubao-seed-1-6", wantBaseURL: "https://ark.cn-beijing.volces.com/api/v3"},
		{name: "qwen", kind: routing.ProviderKindQwen, model: "qwen-plus", wantBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{name: "yuanbao", kind: routing.ProviderKindYuanbao, model: "hunyuan-turbos-latest", wantBaseURL: "https://api.hunyuan.cloud.tencent.com/v1"},
		{name: "kimi", kind: routing.ProviderKindKimi, model: "moonshot-v1-8k", wantBaseURL: "https://api.moonshot.cn/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := pipeline.ResolveGatewayConfigFromSelectedRoute(routing.SelectedProviderRoute{
				Route: routing.ProviderRouteRule{ID: "route", Model: tt.model},
				Credential: routing.ProviderCredential{
					ID:           "cred",
					ProviderKind: tt.kind,
					APIKeyValue:  strPtr("sk-test"),
				},
			}, false, 0)
			if err != nil {
				t.Fatalf("ResolveGatewayConfigFromSelectedRoute() error = %v", err)
			}
			if cfg.ProtocolKind != llm.ProtocolKindOpenAIChatCompletions {
				t.Fatalf("ProtocolKind = %q, want %q", cfg.ProtocolKind, llm.ProtocolKindOpenAIChatCompletions)
			}
			if cfg.Transport.BaseURL != tt.wantBaseURL {
				t.Fatalf("BaseURL = %q, want %q", cfg.Transport.BaseURL, tt.wantBaseURL)
			}
		})
	}
}

func TestRoutingMiddleware_MissingApiKey_Panics(t *testing.T) {
	cfg := routing.ProviderRoutingConfig{
		DefaultRouteID: "route-nokey",
		Credentials: []routing.ProviderCredential{
			{
				ID: "cred-nokey", Name: "nokey-cred",
				OwnerKind: routing.CredentialScopePlatform, ProviderKind: routing.ProviderKindOpenAI,
				APIKeyEnv: strPtr("ARKLOOP_NONEXISTENT_KEY_FOR_TEST"),
			},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-nokey", Model: "gpt-4", CredentialID: "cred-nokey", Multiplier: 1.0},
		},
	}
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	// API key 缺失 -> gatewayFromCredential error -> appendAndCommitSingle(nil pool) -> panic
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("缺失 API key 时应 panic（nil pool）")
		}
	}()

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, _ *pipeline.RunContext) error {
		t.Fatal("不应到达终端 handler")
		return nil
	})
	_ = h(context.Background(), rc)
}

func TestRoutingMiddleware_ResolveGatewayForAgentName_NilDbPool(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		_, _, err := rc.ResolveGatewayForAgentName(context.Background(), "some-agent")
		if err == nil {
			t.Fatal("dbPool 为 nil 时 ResolveGatewayForAgentName 应返回 error")
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoutingMiddleware_ResolveGatewayForAgentName_EmptyFallbackCurrent(t *testing.T) {
	cfg := buildStubRouterConfig()
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		gw, sel, err := rc.ResolveGatewayForAgentName(context.Background(), "")
		if err != nil {
			t.Fatalf("空 agentName 应回退当前路由: %v", err)
		}
		if gw == nil || sel == nil {
			t.Fatal("应返回当前 gateway/route")
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRoutingMiddleware_ResolveGatewayForAgentName_UsesFullSelectorConfigForByok(t *testing.T) {
	cfg := routing.ProviderRoutingConfig{
		DefaultRouteID: "route-default",
		Credentials: []routing.ProviderCredential{
			{
				ID:           "cred-platform",
				Name:         "platform-openai",
				OwnerKind:    routing.CredentialScopePlatform,
				ProviderKind: routing.ProviderKindStub,
			},
			{
				ID:           "cred-user",
				Name:         "byok-openai",
				OwnerKind:    routing.CredentialScopeUser,
				ProviderKind: routing.ProviderKindStub,
			},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-default", Model: "gpt-4o-mini", CredentialID: "cred-platform", Priority: 100},
			{ID: "route-byok", Model: "gpt-5", CredentialID: "cred-user", Priority: 90},
		},
	}
	router := routing.NewProviderRouter(cfg)

	mw := pipeline.NewRoutingMiddleware(
		router, nil, auxGateway{}, false,
		data.RunsRepository{}, data.RunEventsRepository{},
		nil, nil,
	)

	rc := &pipeline.RunContext{
		Emitter:   events.NewEmitter("test"),
		InputJSON: map[string]any{},
	}

	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, rc *pipeline.RunContext) error {
		_, _, err := rc.ResolveGatewayForAgentName(context.Background(), "byok-openai^gpt-5")
		if err == nil {
			t.Fatal("expected BYOK selector to be evaluated and denied when feature is off")
		}
		if got := err.Error(); got != "policy.byok_disabled: BYOK not enabled" {
			t.Fatalf("unexpected selector error: %v", err)
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected middleware error: %v", err)
	}
}
