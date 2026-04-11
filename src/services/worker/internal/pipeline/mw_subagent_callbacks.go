package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"arkloop/services/shared/runkind"
	"arkloop/services/worker/internal/data"
	"github.com/google/uuid"
)

const staleSubAgentCallbackRunKey = "subagent_callback_stale"

func NewSubAgentCallbackMiddleware() RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc == nil || rc.Pool == nil || rc.Run.ThreadID == uuid.Nil {
			return next(ctx, rc)
		}
		callbacks, err := (data.ThreadSubAgentCallbacksRepository{}).ListPendingByThread(ctx, rc.Pool, rc.Run.ThreadID)
		if err != nil {
			return err
		}
		if len(callbacks) == 0 {
			return next(ctx, rc)
		}
		if rc.InputJSON == nil {
			rc.InputJSON = map[string]any{}
		}
		visibleCallbacks, stale := filterVisiblePendingSubAgentCallbacks(callbacks, rc.InputJSON)
		if stale {
			rc.InputJSON[staleSubAgentCallbackRunKey] = true
		} else {
			delete(rc.InputJSON, staleSubAgentCallbackRunKey)
		}
		if len(visibleCallbacks) == 0 {
			rc.PendingSubAgentCallbacks = nil
			delete(rc.InputJSON, "pending_subagent_callbacks")
			return next(ctx, rc)
		}
		rc.PendingSubAgentCallbacks = visibleCallbacks
		rc.InputJSON["pending_subagent_callbacks"] = encodePendingSubAgentCallbacks(visibleCallbacks)
		return next(ctx, rc)
	}
}

func filterVisiblePendingSubAgentCallbacks(callbacks []data.ThreadSubAgentCallbackRecord, inputJSON map[string]any) ([]data.ThreadSubAgentCallbackRecord, bool) {
	runKind := strings.TrimSpace(stringValue(inputJSON["run_kind"]))
	if !strings.EqualFold(runKind, runkind.SubagentCallback) {
		return callbacks, false
	}
	callbackID := parseOptionalUUID(stringValue(inputJSON["callback_id"]))
	if callbackID == nil || *callbackID == uuid.Nil {
		return nil, true
	}
	for _, callback := range callbacks {
		if callback.ID == *callbackID {
			return []data.ThreadSubAgentCallbackRecord{callback}, false
		}
	}
	return nil, true
}

func encodePendingSubAgentCallbacks(callbacks []data.ThreadSubAgentCallbackRecord) []map[string]any {
	out := make([]map[string]any, 0, len(callbacks))
	for _, callback := range callbacks {
		item := map[string]any{
			"callback_id":   callback.ID.String(),
			"sub_agent_id":  callback.SubAgentID.String(),
			"source_run_id": callback.SourceRunID.String(),
			"status":        callback.Status,
		}
		for key, value := range callback.PayloadJSON {
			item[key] = value
		}
		out = append(out, item)
	}
	return out
}

func buildPendingSubAgentCallbacksBlock(callbacks []data.ThreadSubAgentCallbackRecord) string {
	if len(callbacks) == 0 {
		return ""
	}
	encodedCallbacks := encodePendingSubAgentCallbacks(callbacks)
	lines := make([]string, 0, len(callbacks)+2)
	lines = append(lines, "<pending_subagent_callbacks>")
	for i, callback := range encodedCallbacks {
		raw, err := json.Marshal(callback)
		if err != nil {
			lines = append(lines, fmt.Sprintf("- callback_id=%s sub_agent_id=%s status=%s", callbacks[i].ID, callbacks[i].SubAgentID, strings.TrimSpace(callbacks[i].Status)))
			continue
		}
		lines = append(lines, string(raw))
	}
	lines = append(lines, "</pending_subagent_callbacks>")
	return strings.Join(lines, "\n")
}
