package exit_plan_mode

import (
	"context"
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
	binding := &bindingStub{
		active: true,
		path:   "plans/" + threadID.String() + ".md",
	}

	result := New().Execute(context.Background(), ToolName, map[string]any{}, tools.ExecutionContext{
		ThreadID:   &threadID,
		Emitter:    events.NewEmitter("trace"),
		PipelineRC: binding,
	}, "call_1")

	if result.Error != nil {
		t.Fatalf("exit_plan_mode error: %v", result.Error)
	}
	if binding.active {
		t.Fatal("expected binding to exit plan mode")
	}
	if len(result.Events) != 1 || result.Events[0].Type != "thread.plan_mode.updated" {
		t.Fatalf("expected thread.plan_mode.updated event, got %#v", result.Events)
	}
	if got, _ := result.Events[0].DataJSON["plan_mode"].(bool); got {
		t.Fatalf("event plan_mode = %#v, want false", result.Events[0].DataJSON["plan_mode"])
	}
}
