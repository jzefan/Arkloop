package tools

import (
	"strings"

	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/stablejson"
	"github.com/google/uuid"
)

const (
	PolicyDeniedCode = "policy.denied"

	DenyReasonToolNotInAllowlist = "tool.not_in_allowlist"
	DenyReasonToolArgsInvalid    = "tool.args_invalid"
	DenyReasonToolUnknown        = "tool.unknown"
)

type ToolCallDecision struct {
	ToolCallID string
	Allowed    bool
	Events     []events.RunEvent
}

type PolicyEnforcer struct {
	registry  *Registry
	allowlist Allowlist
}

func NewPolicyEnforcer(registry *Registry, allowlist Allowlist) *PolicyEnforcer {
	return &PolicyEnforcer{
		registry:  registry,
		allowlist: allowlist,
	}
}

func (p *PolicyEnforcer) RequestToolCall(
	emitter events.Emitter,
	logicalToolName string,
	argsJSON map[string]any,
	toolCallID string,
	resolvedToolNames ...string,
) ToolCallDecision {
	resolvedToolName := resolvePolicyToolName(logicalToolName, resolvedToolNames...)
	callEvent, resolvedID, argsHash, hasSpec := p.buildToolCallEvent(emitter, logicalToolName, resolvedToolName, argsJSON, toolCallID)
	allowlistName := strings.TrimSpace(logicalToolName)
	if allowlistName == "" {
		allowlistName = strings.TrimSpace(resolvedToolName)
	}

	if argsHash == "" {
		payload := map[string]any{
			"code":         PolicyDeniedCode,
			"message":      "tool arguments invalid",
			"deny_reason":  DenyReasonToolArgsInvalid,
			"tool_call_id": resolvedID,
			"tool_name":    logicalToolName,
			"allowlist":    p.allowlist.ToSortedList(),
		}
		if strings.TrimSpace(resolvedToolName) != "" && strings.TrimSpace(resolvedToolName) != strings.TrimSpace(logicalToolName) {
			payload["resolved_tool_name"] = strings.TrimSpace(resolvedToolName)
		}
		denied := emitter.Emit("policy.denied", payload, stringPtr(logicalToolName), stringPtr(PolicyDeniedCode))
		return ToolCallDecision{
			ToolCallID: resolvedID,
			Allowed:    false,
			Events:     []events.RunEvent{callEvent, denied},
		}
	}

	if !hasSpec {
		payload := map[string]any{
			"code":         PolicyDeniedCode,
			"message":      "tool not registered",
			"deny_reason":  DenyReasonToolUnknown,
			"tool_call_id": resolvedID,
			"tool_name":    logicalToolName,
			"args_hash":    argsHash,
			"allowlist":    p.allowlist.ToSortedList(),
		}
		if strings.TrimSpace(resolvedToolName) != "" && strings.TrimSpace(resolvedToolName) != strings.TrimSpace(logicalToolName) {
			payload["resolved_tool_name"] = strings.TrimSpace(resolvedToolName)
		}
		denied := emitter.Emit("policy.denied", payload, stringPtr(logicalToolName), stringPtr(PolicyDeniedCode))
		return ToolCallDecision{
			ToolCallID: resolvedID,
			Allowed:    false,
			Events:     []events.RunEvent{callEvent, denied},
		}
	}

	allowed := p.allowlist.Allows(allowlistName)
	if !allowed && strings.TrimSpace(resolvedToolName) != "" && strings.TrimSpace(resolvedToolName) != allowlistName {
		allowed = p.allowlist.Allows(strings.TrimSpace(resolvedToolName))
	}
	if !allowed {
		deniedPayload := map[string]any{
			"code":         PolicyDeniedCode,
			"message":      "tool not in allowlist",
			"deny_reason":  DenyReasonToolNotInAllowlist,
			"tool_call_id": resolvedID,
			"tool_name":    logicalToolName,
			"args_hash":    argsHash,
			"allowlist":    p.allowlist.ToSortedList(),
		}
		if strings.TrimSpace(resolvedToolName) != "" && strings.TrimSpace(resolvedToolName) != strings.TrimSpace(logicalToolName) {
			deniedPayload["resolved_tool_name"] = strings.TrimSpace(resolvedToolName)
		}
		if spec, ok := p.registry.Get(resolvedToolName); ok {
			for key, value := range toolCallSpecPayload(spec) {
				deniedPayload[key] = value
			}
		}
		denied := emitter.Emit(
			"policy.denied",
			deniedPayload,
			stringPtr(logicalToolName),
			stringPtr(PolicyDeniedCode),
		)
		return ToolCallDecision{
			ToolCallID: resolvedID,
			Allowed:    false,
			Events:     []events.RunEvent{callEvent, denied},
		}
	}

	return ToolCallDecision{
		ToolCallID: resolvedID,
		Allowed:    true,
		Events:     []events.RunEvent{callEvent},
	}
}

func (p *PolicyEnforcer) BuildToolCallEvent(
	emitter events.Emitter,
	logicalToolName string,
	argsJSON map[string]any,
	toolCallID string,
	resolvedToolNames ...string,
) events.RunEvent {
	resolvedToolName := resolvePolicyToolName(logicalToolName, resolvedToolNames...)
	callEvent, _, _, _ := p.buildToolCallEvent(emitter, logicalToolName, resolvedToolName, argsJSON, toolCallID)
	return callEvent
}

func (p *PolicyEnforcer) buildToolCallEvent(
	emitter events.Emitter,
	logicalToolName string,
	resolvedToolName string,
	argsJSON map[string]any,
	toolCallID string,
) (events.RunEvent, string, string, bool) {
	resolvedID := resolveToolCallID(toolCallID)

	argsHash, err := stablejson.Sha256(argsJSON)
	if err != nil {
		argsHash = ""
	}

	spec, hasSpec := p.registry.Get(resolvedToolName)
	callPayload := map[string]any{
		"tool_call_id": resolvedID,
		"tool_name":    logicalToolName,
		"arguments":    argsJSON,
	}
	if strings.TrimSpace(resolvedToolName) != "" {
		callPayload["resolved_tool_name"] = strings.TrimSpace(resolvedToolName)
	}
	if argsHash != "" {
		callPayload["args_hash"] = argsHash
	}
	if hasSpec {
		for key, value := range toolCallSpecPayload(spec) {
			callPayload[key] = value
		}
	}

	return emitter.Emit("tool.call", callPayload, stringPtr(logicalToolName), nil), resolvedID, argsHash, hasSpec
}

func resolveToolCallID(toolCallID string) string {
	cleaned := strings.TrimSpace(toolCallID)
	if cleaned == "" {
		return uuid.NewString()
	}
	return cleaned
}

func stringPtr(value string) *string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func toolCallSpecPayload(spec AgentToolSpec) map[string]any {
	payload := spec.ToToolCallJSON()
	delete(payload, "tool_name")
	return payload
}

func resolvePolicyToolName(logicalToolName string, resolvedToolNames ...string) string {
	if len(resolvedToolNames) > 0 && strings.TrimSpace(resolvedToolNames[0]) != "" {
		return strings.TrimSpace(resolvedToolNames[0])
	}
	return strings.TrimSpace(logicalToolName)
}
