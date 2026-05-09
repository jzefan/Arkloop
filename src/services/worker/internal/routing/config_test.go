package routing

import "testing"

func TestParseProviderKindDeepSeek(t *testing.T) {
	got, err := parseProviderKind("deepseek")
	if err != nil {
		t.Fatalf("parseProviderKind() error = %v", err)
	}
	if got != ProviderKindDeepSeek {
		t.Fatalf("parseProviderKind() = %q, want %q", got, ProviderKindDeepSeek)
	}
}

func TestDBProviderToKindDeepSeek(t *testing.T) {
	got, err := dbProviderToKind("deepseek")
	if err != nil {
		t.Fatalf("dbProviderToKind() error = %v", err)
	}
	if got != ProviderKindDeepSeek {
		t.Fatalf("dbProviderToKind() = %q, want %q", got, ProviderKindDeepSeek)
	}
}

func TestParseProviderKindZenMax(t *testing.T) {
	got, err := parseProviderKind("zenmax")
	if err != nil {
		t.Fatalf("parseProviderKind() error = %v", err)
	}
	if got != ProviderKindZenMax {
		t.Fatalf("parseProviderKind() = %q, want %q", got, ProviderKindZenMax)
	}
}

func TestDBProviderToKindZenMax(t *testing.T) {
	got, err := dbProviderToKind("zenmax")
	if err != nil {
		t.Fatalf("dbProviderToKind() error = %v", err)
	}
	if got != ProviderKindZenMax {
		t.Fatalf("dbProviderToKind() = %q, want %q", got, ProviderKindZenMax)
	}
}

func TestProviderKindOpenAICompatibleVendors(t *testing.T) {
	tests := []struct {
		raw  string
		want ProviderKind
	}{
		{"doubao", ProviderKindDoubao},
		{"qwen", ProviderKindQwen},
		{"yuanbao", ProviderKindYuanbao},
		{"kimi", ProviderKindKimi},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseProviderKind(tt.raw)
			if err != nil {
				t.Fatalf("parseProviderKind() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseProviderKind() = %q, want %q", got, tt.want)
			}

			got, err = dbProviderToKind(tt.raw)
			if err != nil {
				t.Fatalf("dbProviderToKind() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("dbProviderToKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAvailableModelOptionsUsesExactSelectorsAndFiltersWhen(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-deepseek", Name: "deepseek official", OwnerKind: CredentialScopeUser, ProviderKind: ProviderKindDeepSeek},
			{ID: "cred-qwen", Name: "qwen proxy", OwnerKind: CredentialScopeUser, ProviderKind: ProviderKindOpenAI},
			{ID: "cred-hidden", Name: "doubao hidden", OwnerKind: CredentialScopeUser, ProviderKind: ProviderKindOpenAI},
		},
		Routes: []ProviderRouteRule{
			{ID: "deepseek", Model: "deepseek-chat", CredentialID: "cred-deepseek", When: map[string]any{}},
			{ID: "qwen", Model: "qwen-max", CredentialID: "cred-qwen", When: map[string]any{"persona_id": "industry-education-index"}},
			{ID: "hidden", Model: "doubao-seed-1-6", CredentialID: "cred-hidden", When: map[string]any{"persona_id": "other"}},
		},
	}

	got := cfg.AvailableModelOptions(map[string]any{"persona_id": "industry-education-index"})
	if len(got) != 2 {
		t.Fatalf("expected 2 visible options, got %#v", got)
	}
	values := map[string]bool{}
	for _, item := range got {
		values[item["selector"].(string)] = true
	}
	if !values["deepseek official^deepseek-chat"] || !values["qwen proxy^qwen-max"] {
		t.Fatalf("unexpected selectors: %#v", values)
	}
}

func TestAvailableModelOptionsFiltersHiddenPickerRoutes(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-qwen", Name: "qianwen", OwnerKind: CredentialScopeUser, ProviderKind: ProviderKindOpenAI},
		},
		Routes: []ProviderRouteRule{
			{ID: "enabled", Model: "qwen-plus", CredentialID: "cred-qwen", When: map[string]any{}},
			{ID: "hidden", Model: "qwen-image", CredentialID: "cred-qwen", When: map[string]any{}, HideFromPicker: true},
		},
	}

	got := cfg.AvailableModelOptions(map[string]any{})
	if len(got) != 1 {
		t.Fatalf("expected only enabled picker route, got %#v", got)
	}
	if got[0]["selector"] != "qianwen^qwen-plus" {
		t.Fatalf("unexpected selector: %#v", got[0])
	}
}

func TestGetHighestPriorityRouteByCredentialName_Found(t *testing.T) {
	cfg := ProviderRoutingConfig{
		DefaultRouteID: "default",
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-anthropic", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
			{ID: "cred-b", Name: "my-openai", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "route-b", Model: "gpt-4o", CredentialID: "cred-b", When: map[string]any{}},
			{ID: "route-a", Model: "claude-3", CredentialID: "cred-a", When: map[string]any{}},
		},
	}

	route, cred, ok := cfg.GetHighestPriorityRouteByCredentialName("my-anthropic", map[string]any{})
	if !ok {
		t.Fatal("expected route to be found")
	}
	if route.ID != "route-a" {
		t.Fatalf("expected route-a, got %q", route.ID)
	}
	if cred.ID != "cred-a" {
		t.Fatalf("expected cred-a, got %q", cred.ID)
	}
}

func TestGetHighestPriorityRouteByCredentialName_CaseInsensitive(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "My-Anthropic", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "route-a", Model: "claude-3", CredentialID: "cred-a", When: map[string]any{}},
		},
	}

	_, _, ok := cfg.GetHighestPriorityRouteByCredentialName("my-anthropic", map[string]any{})
	if !ok {
		t.Fatal("expected case-insensitive match")
	}
}

func TestGetHighestPriorityRouteByCredentialName_NotFound(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-anthropic", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "route-a", Model: "claude-3", CredentialID: "cred-a", When: map[string]any{}},
		},
	}

	_, _, ok := cfg.GetHighestPriorityRouteByCredentialName("nonexistent", map[string]any{})
	if ok {
		t.Fatal("expected not found")
	}
}

func TestGetHighestPriorityRouteByCredentialName_EmptyName(t *testing.T) {
	cfg := ProviderRoutingConfig{}
	_, _, ok := cfg.GetHighestPriorityRouteByCredentialName("", map[string]any{})
	if ok {
		t.Fatal("expected false for empty name")
	}
}

func TestGetHighestPriorityRouteByCredentialName_CredentialWithNoRoute(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "orphan-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{},
	}

	_, _, ok := cfg.GetHighestPriorityRouteByCredentialName("orphan-cred", map[string]any{})
	if ok {
		t.Fatal("expected false when credential has no routes")
	}
}

// TestGetHighestPriorityRouteByCredentialName_PrefersWhenMatch 验证同凭证下优先选命中 When 条件的路由。
func TestGetHighestPriorityRouteByCredentialName_PrefersWhenMatch(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-anthropic", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			// 第一条：有 When 条件，匹配特定 persona
			{ID: "route-specific", Model: "claude-3-opus", CredentialID: "cred-a", When: map[string]any{"persona_id": "code-review"}},
			// 第二条：无 When 条件，兜底
			{ID: "route-fallback", Model: "claude-3-haiku", CredentialID: "cred-a", When: map[string]any{}},
		},
	}

	// inputJSON 命中第一条路由的 When 条件
	input := map[string]any{"persona_id": "code-review"}
	route, _, ok := cfg.GetHighestPriorityRouteByCredentialName("my-anthropic", input)
	if !ok {
		t.Fatal("expected route to be found")
	}
	if route.ID != "route-specific" {
		t.Fatalf("expected route-specific (When match), got %q", route.ID)
	}
}

// TestGetHighestPriorityRouteByCredentialName_FallbackWhenNoMatch 验证 When 条件不命中时回退到首条路由。
func TestGetHighestPriorityRouteByCredentialName_FallbackWhenNoMatch(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-anthropic", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "route-specific", Model: "claude-3-opus", CredentialID: "cred-a", When: map[string]any{"persona_id": "code-review"}},
			{ID: "route-fallback", Model: "claude-3-haiku", CredentialID: "cred-a", When: map[string]any{}},
		},
	}

	// inputJSON 不匹配任何 When 条件
	input := map[string]any{"persona_id": "chat"}
	route, _, ok := cfg.GetHighestPriorityRouteByCredentialName("my-anthropic", input)
	if !ok {
		t.Fatal("expected fallback route to be found")
	}
	if route.ID != "route-fallback" {
		t.Fatalf("expected route-fallback (no When match), got %q", route.ID)
	}
}

func TestPickBestRoute_HigherPriorityWins(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "low", Model: "m1", CredentialID: "cred-a", When: map[string]any{}, Priority: 10},
			{ID: "high", Model: "m1", CredentialID: "cred-a", When: map[string]any{}, Priority: 100},
		},
	}
	route, _, ok := cfg.GetHighestPriorityRouteByModel("m1", map[string]any{})
	if !ok {
		t.Fatal("expected route")
	}
	if route.ID != "high" {
		t.Fatalf("expected high-priority route, got %q", route.ID)
	}
}

func TestPickBestRoute_ModelMatchIsCaseSensitive(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "upper", Model: "MiMo-V2.5-Pro", CredentialID: "cred-a", When: map[string]any{}, Priority: 10},
			{ID: "lower", Model: "mimo-v2.5-pro", CredentialID: "cred-a", When: map[string]any{}, Priority: 1},
		},
	}
	route, _, ok := cfg.GetHighestPriorityRouteByModel("mimo-v2.5-pro", map[string]any{})
	if !ok {
		t.Fatal("expected route")
	}
	if route.ID != "lower" {
		t.Fatalf("expected lower route, got %q", route.ID)
	}
}

func TestPickBestRoute_AccountScopedTiebreaker(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "platform", Model: "m1", CredentialID: "cred-a", When: map[string]any{}, Priority: 50, AccountScoped: false},
			{ID: "account", Model: "m1", CredentialID: "cred-a", When: map[string]any{}, Priority: 50, AccountScoped: true},
		},
	}
	route, _, ok := cfg.GetHighestPriorityRouteByModel("m1", map[string]any{})
	if !ok {
		t.Fatal("expected route")
	}
	if route.ID != "account" {
		t.Fatalf("expected account-scoped route at same priority, got %q", route.ID)
	}
}

func TestPickBestRoute_WhenSpecificTiebreaker(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "catchall", Model: "m1", CredentialID: "cred-a", When: map[string]any{}, Priority: 50},
			{ID: "specific", Model: "m1", CredentialID: "cred-a", When: map[string]any{"persona_id": "review"}, Priority: 50},
		},
	}
	route, _, ok := cfg.GetHighestPriorityRouteByModel("m1", map[string]any{"persona_id": "review"})
	if !ok {
		t.Fatal("expected route")
	}
	if route.ID != "specific" {
		t.Fatalf("expected When-specific route at same priority, got %q", route.ID)
	}
}

func TestPickBestRoute_PriorityBeatsWhenSpecificity(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub, AdvancedJSON: map[string]any{}},
		},
		Routes: []ProviderRouteRule{
			{ID: "specific-low", Model: "m1", CredentialID: "cred-a", When: map[string]any{"persona_id": "review"}, Priority: 10},
			{ID: "catchall-high", Model: "m1", CredentialID: "cred-a", When: map[string]any{}, Priority: 100},
		},
	}
	route, _, ok := cfg.GetHighestPriorityRouteByModel("m1", map[string]any{"persona_id": "review"})
	if !ok {
		t.Fatal("expected route")
	}
	if route.ID != "catchall-high" {
		t.Fatalf("expected higher-priority catchall over lower-priority specific, got %q", route.ID)
	}
}

func TestFindCredentialIDByName_ExactMatchFirst(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "plat", Name: "MyKey", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub},
			{ID: "user", Name: "mykey", OwnerKind: CredentialScopeUser, ProviderKind: ProviderKindStub},
		},
	}
	// exact case match should return "user"
	if id := cfg.findCredentialIDByName("mykey"); id != "user" {
		t.Fatalf("expected exact match 'user', got %q", id)
	}
	// exact case match should return "plat"
	if id := cfg.findCredentialIDByName("MyKey"); id != "plat" {
		t.Fatalf("expected exact match 'plat', got %q", id)
	}
}

func TestFindCredentialIDByName_CaseInsensitivePrefersUser(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "plat", Name: "MyKey", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub},
			{ID: "user", Name: "MYKEY", OwnerKind: CredentialScopeUser, ProviderKind: ProviderKindStub},
		},
	}
	// no exact match for "mykey", case-insensitive should prefer user-scoped
	if id := cfg.findCredentialIDByName("mykey"); id != "user" {
		t.Fatalf("expected user-scoped fallback, got %q", id)
	}
}

func TestPickBestRoute_EmptyRoutes(t *testing.T) {
	cfg := ProviderRoutingConfig{
		Credentials: []ProviderCredential{
			{ID: "cred-a", Name: "my-cred", OwnerKind: CredentialScopePlatform, ProviderKind: ProviderKindStub},
		},
		Routes: []ProviderRouteRule{},
	}
	_, _, ok := cfg.GetHighestPriorityRouteByModel("m1", map[string]any{})
	if ok {
		t.Fatal("expected no route for empty routes")
	}
}
