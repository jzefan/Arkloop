package catalogapi

import (
	"testing"

	"arkloop/services/api/internal/personas"
)

func TestRepoPersonaSettingsVisibleDefaultsToTrue(t *testing.T) {
	if !repoPersonaSettingsVisible(personas.RepoPersona{ID: "industry-education-index"}) {
		t.Fatal("expected repo persona to be visible by default")
	}
}

func TestRepoPersonaSettingsVisibleCanHideHelperPersonas(t *testing.T) {
	hidden := false
	if repoPersonaSettingsVisible(personas.RepoPersona{
		ID:              "industry-education-evaluator",
		SettingsVisible: &hidden,
	}) {
		t.Fatal("expected settings_visible=false persona to be hidden")
	}
}
