package tools

import "github.com/google/uuid"

func ToolOutputScopeID(threadID *uuid.UUID, runID uuid.UUID) string {
	if threadID != nil {
		return threadID.String()
	}
	return runID.String()
}
