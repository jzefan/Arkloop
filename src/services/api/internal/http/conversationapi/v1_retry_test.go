package conversationapi

import (
	"testing"

	"arkloop/services/api/internal/data"
)

func TestInheritRetryRunExecutionDataCopiesParentStartedFields(t *testing.T) {
	startedData := map[string]any{"source": "retry"}
	jobData := map[string]any{"source": "retry"}
	parentStartedData := map[string]any{
		"model":          "provider^parent",
		"persona_id":     "ops@1",
		"role":           "worker",
		"reasoning_mode": "high",
		"route_id":       "primary",
		"work_dir":       "/workspace/project",
	}

	inheritRetryRunExecutionData(startedData, jobData, parentStartedData, nil, nil)

	assertRunField(t, startedData, "persona_id", "ops@1")
	assertRunField(t, jobData, "persona_id", "ops@1")
	assertRunField(t, startedData, "role", "worker")
	assertRunField(t, jobData, "role", "worker")
	assertRunField(t, startedData, "reasoning_mode", "high")
	assertRunField(t, jobData, "reasoning_mode", "high")
	assertRunField(t, startedData, "route_id", "primary")
	assertRunField(t, jobData, "work_dir", "/workspace/project")
}

func TestInheritRetryRunExecutionDataModelOverrideWins(t *testing.T) {
	startedData := map[string]any{"source": "retry"}
	jobData := map[string]any{"source": "retry"}
	parentStartedData := map[string]any{
		"model":      "provider^parent",
		"persona_id": "ops@1",
		"role":       "worker",
	}
	parentRunModel := "provider^persisted"
	requestedModel := "provider^requested"

	inheritRetryRunExecutionData(
		startedData,
		jobData,
		parentStartedData,
		&data.Run{Model: &parentRunModel},
		&createRunRequest{Model: &requestedModel},
	)

	assertRunField(t, startedData, "model", requestedModel)
	assertRunField(t, jobData, "model", requestedModel)
	assertRunField(t, startedData, "persona_id", "ops@1")
	assertRunField(t, jobData, "role", "worker")
}

func TestInheritRetryRunExecutionDataFallsBackToParentRunMetadata(t *testing.T) {
	startedData := map[string]any{"source": "retry"}
	jobData := map[string]any{"source": "retry"}
	parentRunModel := "provider^persisted"
	parentPersonaID := "work"

	inheritRetryRunExecutionData(
		startedData,
		jobData,
		map[string]any{},
		&data.Run{Model: &parentRunModel, PersonaID: &parentPersonaID},
		nil,
	)

	assertRunField(t, startedData, "model", parentRunModel)
	assertRunField(t, jobData, "model", parentRunModel)
	assertRunField(t, startedData, "persona_id", parentPersonaID)
	assertRunField(t, jobData, "persona_id", parentPersonaID)
}

func assertRunField(t *testing.T, values map[string]any, key string, want string) {
	t.Helper()
	if got, _ := values[key].(string); got != want {
		t.Fatalf("%s = %#v, want %q", key, values[key], want)
	}
}
