package loadskill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"arkloop/services/shared/skillstore"
	"arkloop/services/worker/internal/tools"
)

func TestLoadSkillEnabledSkill(t *testing.T) {
	root := t.TempDir()
	skill := skillstore.ResolvedSkill{
		SkillKey:        "grep-helper",
		DisplayName:     "Grep Helper",
		Description:     "Use grep carefully",
		Version:         "1",
		MountPath:       filepath.Join(root, "grep-helper@1"),
		InstructionPath: "SKILL.md",
	}
	if err := os.MkdirAll(skill.MountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill.MountPath, "SKILL.md"), []byte("skill body"), 0o644); err != nil {
		t.Fatal(err)
	}

	exec := NewToolExecutor(nil)
	result := exec.Execute(context.Background(), ToolName, map[string]any{"skill": "grep-helper@1"}, tools.ExecutionContext{
		EnabledSkills: []skillstore.ResolvedSkill{skill},
	}, "")
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if got := result.ResultJSON["content"]; got != "skill body" {
		t.Fatalf("unexpected content: %v", got)
	}
}

func TestLoadSkillExternalSkill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "frontend-design")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("external body"), 0o644); err != nil {
		t.Fatal(err)
	}

	exec := NewToolExecutor(nil)
	result := exec.Execute(context.Background(), ToolName, map[string]any{"skill": "frontend-design"}, tools.ExecutionContext{
		ExternalSkills: []skillstore.ExternalSkill{{
			Name:            "frontend-design",
			Path:            skillDir,
			InstructionPath: "SKILL.md",
			Description:     "Fancy UI",
		}},
	}, "")
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if got := result.ResultJSON["source"]; got != "external" {
		t.Fatalf("unexpected source: %v", got)
	}
}

func TestLoadSkillMissingSkill(t *testing.T) {
	exec := NewToolExecutor(nil)
	result := exec.Execute(context.Background(), ToolName, map[string]any{"skill": "missing"}, tools.ExecutionContext{}, "")
	if result.Error == nil || result.Error.ErrorClass != errorNotFound {
		t.Fatalf("expected not found error, got %+v", result.Error)
	}
}
