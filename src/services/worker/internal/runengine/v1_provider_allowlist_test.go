//go:build !desktop

package runengine

import (
	"testing"

	"arkloop/services/worker/internal/tools"
	webfetch "arkloop/services/worker/internal/tools/builtin/web_fetch"
	websearch "arkloop/services/worker/internal/tools/builtin/web_search"
)

func TestRestoreRuntimeProviderGroupsKeepsLogicalWebTools(t *testing.T) {
	registry := tools.NewRegistry()
	for _, spec := range []tools.AgentToolSpec{
		websearch.AgentSpec,
		websearch.AgentSpecSearxng,
		websearch.AgentSpecTavily,
		webfetch.AgentSpec,
		webfetch.AgentSpecBasic,
		{Name: "timeline_title", Version: "1", Description: "title", RiskLevel: tools.RiskLevelLow},
	} {
		if err := registry.Register(spec); err != nil {
			t.Fatalf("register spec: %v", err)
		}
	}

	original := map[string]struct{}{
		"web_search":        {},
		"web_search.searxng": {},
		"web_search.tavily":  {},
		"web_fetch.basic":    {},
		"timeline_title":     {},
	}
	filtered := map[string]struct{}{"timeline_title": {}}

	got := restoreRuntimeProviderGroups(original, filtered, registry)
	for _, want := range []string{"web_search", "web_fetch", "timeline_title"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("expected %s to be preserved, got %#v", want, got)
		}
	}
	for _, denied := range []string{"web_search.searxng", "web_search.tavily", "web_fetch.basic"} {
		if _, ok := got[denied]; ok {
			t.Fatalf("expected provider variant %s to stay out of base allowlist, got %#v", denied, got)
		}
	}
}
