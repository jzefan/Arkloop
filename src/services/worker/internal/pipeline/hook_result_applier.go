package pipeline

import (
	"sort"
	"strings"
)

type HookResultApplier interface {
	ApplyCompactHints(input CompactInput, hints CompactHints) CompactInput
}

type DefaultHookResultApplier struct{}

func NewDefaultHookResultApplier() HookResultApplier {
	return DefaultHookResultApplier{}
}

func (DefaultHookResultApplier) ApplyCompactHints(input CompactInput, hints CompactHints) CompactInput {
	normalized := sortCompactHints(hints)
	if len(normalized) == 0 {
		return input
	}
	lines := make([]string, 0, len(normalized))
	for _, hint := range normalized {
		content := strings.TrimSpace(hint.Content)
		if content == "" {
			continue
		}
		lines = append(lines, content)
	}
	if len(lines) == 0 {
		return input
	}
	block := "<compact_hints>\n" + strings.Join(lines, "\n") + "\n</compact_hints>"
	prompt := strings.TrimSpace(input.SystemPrompt)
	if prompt == "" {
		input.SystemPrompt = block
		return input
	}
	input.SystemPrompt = prompt + "\n\n" + block
	return input
}

func BuildCompactHintsBlock(hints CompactHints) string {
	out := DefaultHookResultApplier{}.ApplyCompactHints(CompactInput{}, hints)
	return strings.TrimSpace(out.SystemPrompt)
}

func sortCompactHints(hints CompactHints) CompactHints {
	filtered := make(CompactHints, 0, len(hints))
	for _, hint := range hints {
		if strings.TrimSpace(hint.Content) == "" {
			continue
		}
		filtered = append(filtered, hint)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Priority < filtered[j].Priority
	})
	return filtered
}
