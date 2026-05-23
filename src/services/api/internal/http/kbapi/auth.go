package kbapi

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

// membershipChecker is the minimal interface auth.go consumes. The real
// implementation is a thin wrapper around WorkspaceRegistriesRepository +
// AccountMembershipRepository in Tasks 6/7 wiring; the test replaces it
// with a fake.
type membershipChecker interface {
	IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, workspaceRef string) (bool, error)
}

var errNoWorkspaceAccess = errors.New("user has no access to this workspace")

// ensureWorkspaceMember returns nil if the actor's account is a member of
// the workspace; otherwise errNoWorkspaceAccess.
func ensureWorkspaceMember(ctx context.Context, c membershipChecker, accountID uuid.UUID, workspaceRef string) error {
	ok, err := c.IsWorkspaceMember(ctx, accountID, workspaceRef)
	if err != nil {
		return fmt.Errorf("workspace membership check: %w", err)
	}
	if !ok {
		return errNoWorkspaceAccess
	}
	return nil
}

// ensureKBVisible enforces KB.Visibility on top of workspace membership.
// Returns true if the actor can see the KB; otherwise writes 404 and returns false.
func ensureKBVisible(w nethttp.ResponseWriter, kb *data.KnowledgeBase, a actor) bool {
	if kb.Visibility != "private" {
		return true
	}
	if kb.CreatedBy != nil && *kb.CreatedBy == a.UserID {
		return true
	}
	writeErr(w, nethttp.StatusNotFound, "kb.not_found", "kb not found")
	return false
}
