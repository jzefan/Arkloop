package llmproviders

import (
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

func TestMatchProviderRouteBySelectorPrefersCredentialName(t *testing.T) {
	credID := uuid.New()
	providers := []Provider{
		{
			Credential: data.LlmCredential{
				ID:       credID,
				Name:     "openrouter",
				Provider: "openai",
			},
			Models: []data.LlmRoute{
				{CredentialID: credID, Model: "openai/gpt-4o-mini"},
			},
		},
	}

	match, ok, err := matchProviderRouteBySelector(providers, "openrouter^openai/gpt-4o-mini")
	if err != nil {
		t.Fatalf("matchProviderRouteBySelector() error = %v", err)
	}
	if !ok {
		t.Fatal("expected exact selector match")
	}
	if match.provider.Credential.Name != "openrouter" {
		t.Fatalf("unexpected credential name %q", match.provider.Credential.Name)
	}
	if match.route.Model != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected model %q", match.route.Model)
	}
}

func TestMatchProviderRouteBySelectorSupportsLegacyProviderSelector(t *testing.T) {
	credID := uuid.New()
	providers := []Provider{
		{
			Credential: data.LlmCredential{
				ID:       credID,
				Name:     "openrouter",
				Provider: "openai",
			},
			Models: []data.LlmRoute{
				{CredentialID: credID, Model: "openai/text-embedding-3-small"},
			},
		},
	}

	match, ok, err := matchProviderRouteBySelector(providers, "openai^openai/text-embedding-3-small")
	if err != nil {
		t.Fatalf("matchProviderRouteBySelector() error = %v", err)
	}
	if !ok {
		t.Fatal("expected legacy selector match")
	}
	if match.provider.Credential.Name != "openrouter" {
		t.Fatalf("unexpected credential name %q", match.provider.Credential.Name)
	}
}

func TestMatchProviderRouteBySelectorRequiresExactModelCase(t *testing.T) {
	credID := uuid.New()
	providers := []Provider{
		{
			Credential: data.LlmCredential{
				ID:       credID,
				Name:     "moonshot",
				Provider: "openai",
			},
			Models: []data.LlmRoute{
				{CredentialID: credID, Model: "MiMo-V2.5-Pro"},
				{CredentialID: credID, Model: "mimo-v2.5-pro"},
			},
		},
	}

	match, ok, err := matchProviderRouteBySelector(providers, "moonshot^mimo-v2.5-pro")
	if err != nil {
		t.Fatalf("matchProviderRouteBySelector() error = %v", err)
	}
	if !ok {
		t.Fatal("expected exact-case model match")
	}
	if match.route.Model != "mimo-v2.5-pro" {
		t.Fatalf("unexpected model %q", match.route.Model)
	}
}

func TestInferKnownEmbeddingDimension(t *testing.T) {
	dimension := inferKnownEmbeddingDimension("openai/text-embedding-3-small")
	if dimension != 1536 {
		t.Fatalf("dimension = %d, want 1536", dimension)
	}
}
