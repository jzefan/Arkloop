package tools

import (
	"testing"
	"time"
)

type planModePathStub struct {
	active  bool
	allowed string
}

func (s planModePathStub) IsPlanModeActive() bool {
	return s.active
}

func (s planModePathStub) PlanModeWritePathAllowed(path string) bool {
	return path == s.allowed
}

func TestPlanModeWriteBlockedAllowsCurrentPlanFile(t *testing.T) {
	if _, blocked := PlanModeWriteBlocked(planModePathStub{active: true, allowed: "plans/thread.md"}, time.Now(), "plans/thread.md"); blocked {
		t.Fatal("expected current plan file to be allowed")
	}
}

func TestPlanModeWriteBlockedRejectsOtherFiles(t *testing.T) {
	result, blocked := PlanModeWriteBlocked(planModePathStub{active: true, allowed: "plans/thread.md"}, time.Now(), "src/main.go")
	if !blocked {
		t.Fatal("expected non-plan file to be blocked")
	}
	if result.Error == nil {
		t.Fatal("expected blocking error")
	}
}

func TestPlanModeWriteBlockedInactive(t *testing.T) {
	if _, blocked := PlanModeWriteBlocked(planModePathStub{active: false}, time.Now(), "src/main.go"); blocked {
		t.Fatal("expected inactive plan mode to allow writes")
	}
}
