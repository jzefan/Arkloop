package pipeline

import (
	"testing"

	"arkloop/services/worker/internal/routing"
)

func TestResolveSelectedRouteBySelectorSupportsProviderKindLegacySelector(t *testing.T) {
	cfg := routing.ProviderRoutingConfig{
		Credentials: []routing.ProviderCredential{
			{ID: "cred-deepseek", Name: "deepseek official", OwnerKind: routing.CredentialScopeUser, ProviderKind: routing.ProviderKindDeepSeek},
		},
		Routes: []routing.ProviderRouteRule{
			{ID: "route-deepseek", Model: "deepseek-v4-flash", CredentialID: "cred-deepseek", When: map[string]any{}},
		},
	}

	selected, err := resolveSelectedRouteBySelector(cfg, "deepseek^deepseek-v4-flash", map[string]any{}, true)
	if err != nil {
		t.Fatalf("resolveSelectedRouteBySelector() error = %v", err)
	}
	if selected == nil {
		t.Fatal("expected selected route")
	}
	if selected.Route.ID != "route-deepseek" {
		t.Fatalf("expected route-deepseek, got %q", selected.Route.ID)
	}
	if selected.Credential.Name != "deepseek official" {
		t.Fatalf("expected deepseek official credential, got %q", selected.Credential.Name)
	}
}
