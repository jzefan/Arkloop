package conversationapi

import "testing"

func TestApplyEditRunRequestOverridesWins(t *testing.T) {
	startedData := map[string]any{
		"source":     "edit",
		"persona_id": "normal",
		"model":      "provider^old",
	}
	jobData := map[string]any{
		"source":     "edit",
		"persona_id": "normal",
		"model":      "provider^old",
	}
	personaID := "work"
	model := "provider^new"
	reasoningMode := "xhigh"

	if err := applyEditRunRequestOverrides(startedData, jobData, createMessageRequest{
		PersonaID:     &personaID,
		Model:         &model,
		ReasoningMode: &reasoningMode,
	}); err != nil {
		t.Fatalf("applyEditRunRequestOverrides: %v", err)
	}

	assertRunField(t, startedData, "persona_id", personaID)
	assertRunField(t, jobData, "persona_id", personaID)
	assertRunField(t, startedData, "model", model)
	assertRunField(t, jobData, "model", model)
	assertRunField(t, startedData, "reasoning_mode", reasoningMode)
}

func TestSetRunCollaborationModeWritesStartedAndJobData(t *testing.T) {
	startedData := map[string]any{}
	jobData := map[string]any{}

	setRunCollaborationMode(startedData, jobData, "plan", 3)

	if got, _ := startedData["collaboration_mode"].(string); got != "plan" {
		t.Fatalf("started collaboration_mode = %#v, want plan", startedData["collaboration_mode"])
	}
	if got, _ := jobData["collaboration_mode"].(string); got != "plan" {
		t.Fatalf("job collaboration_mode = %#v, want plan", jobData["collaboration_mode"])
	}
	if got, _ := startedData["collaboration_mode_revision"].(int64); got != 3 {
		t.Fatalf("started collaboration_mode_revision = %#v, want 3", startedData["collaboration_mode_revision"])
	}
	if got, _ := jobData["collaboration_mode_revision"].(int64); got != 3 {
		t.Fatalf("job collaboration_mode_revision = %#v, want 3", jobData["collaboration_mode_revision"])
	}
}

func assertRunField(t *testing.T, values map[string]any, key string, want string) {
	t.Helper()
	if got, _ := values[key].(string); got != want {
		t.Fatalf("%s = %#v, want %q", key, values[key], want)
	}
}
