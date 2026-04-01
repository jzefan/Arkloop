package loadskill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
	"arkloop/services/shared/skillstore"
	"arkloop/services/worker/internal/tools"
)

const (
	errorArgsInvalid = "tool.args_invalid"
	errorNotFound    = "tool.not_found"
	errorLoadFailed  = "tool.execution_failed"
)

type ToolExecutor struct {
	store objectstore.Store
}

func NewToolExecutor(store objectstore.Store) *ToolExecutor {
	return &ToolExecutor{store: store}
}

func (e *ToolExecutor) Execute(
	_ context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()

	rawSkill, _ := args["skill"].(string)
	skillName := strings.TrimSpace(rawSkill)
	if skillName == "" {
		return errorResult(started, errorArgsInvalid, "skill is required")
	}

	entry, err := e.findSkillEntry(execCtx, skillName)
	if err != nil {
		class := errorNotFound
		if strings.Contains(err.Error(), "bundle") || strings.Contains(err.Error(), "materialized") || strings.Contains(err.Error(), "write skill file") {
			class = errorLoadFailed
		}
		return errorResult(started, class, err.Error())
	}

	content, err := os.ReadFile(entry.InstructionFile)
	if err != nil {
		return errorResult(started, errorLoadFailed, fmt.Sprintf("read skill %q: %v", entry.Name, err))
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"skill":            entry.Name,
			"display_name":     entry.DisplayName,
			"description":      entry.Description,
			"source":           entry.Source,
			"instruction_path": entry.RelativeInstructionPath,
			"content":          string(content),
		},
		DurationMs: durationMs(started),
	}
}

type skillEntry struct {
	Name                    string
	DisplayName             string
	Description             string
	Source                  string
	InstructionFile         string
	RelativeInstructionPath string
}

func (e *ToolExecutor) findSkillEntry(execCtx tools.ExecutionContext, skillName string) (skillEntry, error) {
	name := strings.TrimSpace(skillName)
	lowerName := strings.ToLower(name)
	for _, item := range execCtx.EnabledSkills {
		candidateNames := []string{
			strings.TrimSpace(item.SkillKey),
			strings.TrimSpace(item.DisplayName),
			formatEnabledSkillName(item),
		}
		if !matchesSkillName(lowerName, candidateNames...) {
			continue
		}
		instructionPath := strings.TrimSpace(item.InstructionPath)
		if instructionPath == "" {
			instructionPath = skillstore.InstructionPathDefault
		}
		instructionFile, err := ensureInstructionFile(item, instructionPath, e.store)
		if err != nil {
			return skillEntry{}, err
		}
		return skillEntry{
			Name:                    strings.TrimSpace(item.SkillKey),
			DisplayName:             strings.TrimSpace(item.DisplayName),
			Description:             strings.TrimSpace(item.Description),
			Source:                  "enabled",
			InstructionFile:         instructionFile,
			RelativeInstructionPath: instructionPath,
		}, nil
	}

	for _, item := range execCtx.ExternalSkills {
		candidateNames := []string{strings.TrimSpace(item.Name)}
		if !matchesSkillName(lowerName, candidateNames...) {
			continue
		}
		instructionPath := strings.TrimSpace(item.InstructionPath)
		if instructionPath == "" {
			instructionPath = skillstore.InstructionPathDefault
		}
		return skillEntry{
			Name:                    strings.TrimSpace(item.Name),
			DisplayName:             strings.TrimSpace(item.Name),
			Description:             strings.TrimSpace(item.Description),
			Source:                  "external",
			InstructionFile:         filepath.Join(strings.TrimSpace(item.Path), filepath.FromSlash(instructionPath)),
			RelativeInstructionPath: instructionPath,
		}, nil
	}

	return skillEntry{}, fmt.Errorf("skill %q is not available in this run; available skills: %s", skillName, strings.Join(availableSkillNames(execCtx), ", "))
}

func ensureInstructionFile(skill skillstore.ResolvedSkill, instructionPath string, store objectstore.Store) (string, error) {
	instructionFile := filepath.Join(strings.TrimSpace(skill.MountPath), filepath.FromSlash(instructionPath))
	if _, err := os.Stat(instructionFile); err == nil {
		return instructionFile, nil
	}
	if store == nil {
		return "", fmt.Errorf("skill %q is not materialized on disk", strings.TrimSpace(skill.SkillKey))
	}
	bundleRef := strings.TrimSpace(skill.BundleRef)
	if bundleRef == "" {
		return "", fmt.Errorf("skill %q bundle_ref is empty", strings.TrimSpace(skill.SkillKey))
	}
	encoded, err := store.Get(context.Background(), bundleRef)
	if err != nil {
		return "", fmt.Errorf("load skill bundle %q: %w", strings.TrimSpace(skill.SkillKey), err)
	}
	bundle, err := skillstore.DecodeBundle(encoded)
	if err != nil {
		return "", fmt.Errorf("decode skill bundle %q: %w", strings.TrimSpace(skill.SkillKey), err)
	}
	root := strings.TrimSpace(skill.MountPath)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create skill dir %q: %w", root, err)
	}
	for _, file := range bundle.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		target = filepath.Clean(target)
		if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return "", fmt.Errorf("skill file escapes root: %s", file.Path)
		}
		if file.IsDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", fmt.Errorf("create skill dir %q: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", fmt.Errorf("create skill parent %q: %w", target, err)
		}
		mode := os.FileMode(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, file.Data, mode); err != nil {
			return "", fmt.Errorf("write skill file %q: %w", target, err)
		}
	}
	return instructionFile, nil
}

func matchesSkillName(query string, names ...string) bool {
	for _, name := range names {
		if strings.EqualFold(strings.TrimSpace(name), query) {
			return true
		}
	}
	return false
}

func formatEnabledSkillName(item skillstore.ResolvedSkill) string {
	key := strings.TrimSpace(item.SkillKey)
	version := strings.TrimSpace(item.Version)
	if key == "" || version == "" {
		return key
	}
	return key + "@" + version
}

func availableSkillNames(execCtx tools.ExecutionContext) []string {
	names := make([]string, 0, len(execCtx.EnabledSkills))
	seen := map[string]struct{}{}
	for _, item := range execCtx.EnabledSkills {
		name := formatEnabledSkillName(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, item := range execCtx.ExternalSkills {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func errorResult(started time.Time, class, message string) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: class,
			Message:    message,
		},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	ms := int(time.Since(started) / time.Millisecond)
	if ms < 0 {
		return 0
	}
	return ms
}
