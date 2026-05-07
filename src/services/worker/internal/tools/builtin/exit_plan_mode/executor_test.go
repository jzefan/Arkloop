package exit_plan_mode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type bindingStub struct {
	active bool
	path   string
}

func (b *bindingStub) SetIsPlanMode(active bool) {
	b.active = active
}

func (b *bindingStub) PlanFilePathValue() string {
	return b.path
}

func (b *bindingStub) IsPlanModeActive() bool {
	return b.active
}

func TestExitPlanModeDoesNotRequirePlanArgument(t *testing.T) {
	threadID := uuid.New()
	workDir := t.TempDir()
	planPath := "plans/" + threadID.String() + ".md"
	if err := os.MkdirAll(filepath.Join(workDir, "plans"), 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, planPath), []byte("1. inspect\n2. implement\n"), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	binding := &bindingStub{
		active: true,
		path:   planPath,
	}

	result := New().Execute(context.Background(), ToolName, map[string]any{}, tools.ExecutionContext{
		ThreadID:   &threadID,
		RunID:      uuid.New(),
		WorkDir:    workDir,
		Emitter:    events.NewEmitter("trace"),
		PipelineRC: binding,
	}, "call_1")

	if result.Error != nil {
		t.Fatalf("exit_plan_mode error: %v", result.Error)
	}
	if binding.active {
		t.Fatal("expected binding to exit plan mode")
	}
	if len(result.Events) != 1 || result.Events[0].Type != "thread.collaboration_mode.updated" {
		t.Fatalf("expected thread.collaboration_mode.updated event, got %#v", result.Events)
	}
	if got, _ := result.Events[0].DataJSON["collaboration_mode"].(string); got != "default" {
		t.Fatalf("event collaboration_mode = %#v, want default", result.Events[0].DataJSON["collaboration_mode"])
	}
}
