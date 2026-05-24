package skillstore

import "testing"

func TestValidateItemWritingSkill_NonItemWritingSkillIsAlwaysValid(t *testing.T) {
	// Plain skill (no category) should not be subject to item-writing checks.
	def := SkillDefinition{
		SkillKey:    "regular-skill",
		Version:     "1",
		DisplayName: "Regular",
	}
	if err := ValidateItemWritingSkill(def); err != nil {
		t.Errorf("non-item-writing skill should be valid, got: %v", err)
	}
}

func TestValidateItemWritingSkill_RequiresPatternTags(t *testing.T) {
	def := SkillDefinition{
		SkillKey:            "broken",
		Version:             "1",
		DisplayName:         "Broken",
		Category:            "item-writing",
		TargetQuestionTypes: []string{"single_choice"},
		// PatternTags missing
	}
	if err := ValidateItemWritingSkill(def); err == nil {
		t.Error("expected error when pattern_tags missing")
	}
}

func TestValidateItemWritingSkill_RequiresTargetQuestionTypes(t *testing.T) {
	def := SkillDefinition{
		SkillKey:    "broken",
		Version:     "1",
		DisplayName: "Broken",
		Category:    "item-writing",
		PatternTags: []string{"A1", "A2"},
		// TargetQuestionTypes missing
	}
	if err := ValidateItemWritingSkill(def); err == nil {
		t.Error("expected error when target_question_types missing")
	}
}

func TestValidateItemWritingSkill_AcceptsCompleteDefinition(t *testing.T) {
	def := SkillDefinition{
		SkillKey:            "gyn-medical-exam",
		Version:             "1",
		DisplayName:         "妇产科国考命题专家",
		Category:            "item-writing",
		SubjectTags:         []string{"妇产科", "医学"},
		PatternTags:         []string{"A1", "A2", "A3", "A4"},
		TargetQuestionTypes: []string{"single_choice"},
	}
	if err := ValidateItemWritingSkill(def); err != nil {
		t.Errorf("complete definition should pass, got: %v", err)
	}
}
