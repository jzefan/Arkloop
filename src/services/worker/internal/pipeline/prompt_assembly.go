package pipeline

import (
	"strings"
)

type PromptSegmentTarget string

const (
	PromptTargetSystemPrefix       PromptSegmentTarget = "system_prefix"
	PromptTargetConversationPrefix PromptSegmentTarget = "conversation_prefix"
	PromptTargetRuntimeTail        PromptSegmentTarget = "runtime_tail"
)

type PromptSegmentStability string

const (
	PromptStabilityStablePrefix  PromptSegmentStability = "stable_prefix"
	PromptStabilitySessionPrefix PromptSegmentStability = "session_prefix"
	PromptStabilityVolatileTail  PromptSegmentStability = "volatile_tail"
)

type PromptSegment struct {
	Name          string
	Target        PromptSegmentTarget
	Role          string
	Text          string
	Stability     PromptSegmentStability
	CacheEligible bool
}

type PromptAssembly struct {
	Segments []PromptSegment
}

func (a PromptAssembly) Clone() PromptAssembly {
	if len(a.Segments) == 0 {
		return PromptAssembly{}
	}
	out := make([]PromptSegment, len(a.Segments))
	copy(out, a.Segments)
	return PromptAssembly{Segments: out}
}

func (a *PromptAssembly) Reset() {
	if a == nil {
		return
	}
	a.Segments = nil
}

func (a *PromptAssembly) Upsert(segment PromptSegment) {
	if a == nil {
		return
	}
	normalized, ok := normalizePromptSegment(segment)
	if !ok {
		return
	}
	for i := range a.Segments {
		if a.Segments[i].Name == normalized.Name {
			a.Segments[i] = normalized
			return
		}
	}
	a.Segments = append(a.Segments, normalized)
}

func (a *PromptAssembly) Append(segment PromptSegment) {
	if a == nil {
		return
	}
	normalized, ok := normalizePromptSegment(segment)
	if !ok {
		return
	}
	a.Segments = append(a.Segments, normalized)
}

func (a *PromptAssembly) Remove(name string) {
	if a == nil {
		return
	}
	target := strings.TrimSpace(name)
	if target == "" || len(a.Segments) == 0 {
		return
	}
	filtered := a.Segments[:0]
	for _, segment := range a.Segments {
		if segment.Name == target {
			continue
		}
		filtered = append(filtered, segment)
	}
	a.Segments = filtered
}

func (a *PromptAssembly) RemoveByPrefix(prefix string) {
	if a == nil {
		return
	}
	target := strings.TrimSpace(prefix)
	if target == "" || len(a.Segments) == 0 {
		return
	}
	filtered := a.Segments[:0]
	for _, segment := range a.Segments {
		if strings.HasPrefix(segment.Name, target) {
			continue
		}
		filtered = append(filtered, segment)
	}
	a.Segments = filtered
}

func (a PromptAssembly) MaterializeSystemPrompt() string {
	parts := make([]string, 0, len(a.Segments))
	for _, segment := range a.Segments {
		if segment.Role != "system" {
			continue
		}
		if segment.Target != PromptTargetSystemPrefix && segment.Target != PromptTargetConversationPrefix {
			continue
		}
		parts = append(parts, strings.TrimSpace(segment.Text))
	}
	return strings.Join(parts, "\n\n")
}

func (a PromptAssembly) MaterializeRuntimePrompt() string {
	parts := make([]string, 0, len(a.Segments))
	for _, segment := range a.Segments {
		if segment.Target != PromptTargetRuntimeTail {
			continue
		}
		if segment.Role != "user" {
			continue
		}
		parts = append(parts, strings.TrimSpace(segment.Text))
	}
	return strings.Join(parts, "\n\n")
}

func normalizePromptSegment(segment PromptSegment) (PromptSegment, bool) {
	segment.Name = strings.TrimSpace(segment.Name)
	segment.Role = strings.TrimSpace(segment.Role)
	segment.Text = strings.TrimSpace(segment.Text)
	if segment.Name == "" || segment.Role == "" || segment.Text == "" {
		return PromptSegment{}, false
	}
	switch segment.Target {
	case PromptTargetSystemPrefix, PromptTargetConversationPrefix, PromptTargetRuntimeTail:
	default:
		segment.Target = PromptTargetSystemPrefix
	}
	switch segment.Stability {
	case PromptStabilityStablePrefix, PromptStabilitySessionPrefix, PromptStabilityVolatileTail:
	default:
		segment.Stability = PromptStabilitySessionPrefix
	}
	return segment, true
}
