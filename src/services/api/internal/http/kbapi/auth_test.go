package kbapi

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeMembershipChecker implements the small surface area auth.go uses.
// The integration with the real repos is exercised end-to-end in Task 14.
type fakeMembershipChecker struct {
	memberOf map[string]bool // key = "<accountID>/<workspaceRef>"
}

func (f *fakeMembershipChecker) IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, workspaceRef string) (bool, error) {
	return f.memberOf[accountID.String()+"/"+workspaceRef], nil
}

func TestEnsureWorkspaceMemberAllows(t *testing.T) {
	acc := uuid.New()
	checker := &fakeMembershipChecker{memberOf: map[string]bool{acc.String() + "/ws-1": true}}
	if err := ensureWorkspaceMember(context.Background(), checker, acc, "ws-1"); err != nil {
		t.Errorf("unexpected denial: %v", err)
	}
}

func TestEnsureWorkspaceMemberDenies(t *testing.T) {
	acc := uuid.New()
	checker := &fakeMembershipChecker{memberOf: map[string]bool{}}
	err := ensureWorkspaceMember(context.Background(), checker, acc, "ws-2")
	if err == nil {
		t.Error("expected denial")
	}
}
