package askuser

import (
	"testing"
)

func TestValidateQuestions_Valid(t *testing.T) {
	valid := map[string]any{
		"questions": []any{
			map[string]any{
				"id":       "q1",
				"question": "Pick one",
				"options": []any{
					map[string]any{"value": "a", "label": "Option A"},
					map[string]any{"value": "b", "label": "Option B", "description": "desc", "recommended": true},
				},
			},
		},
	}
	if _, err := validateQuestions(valid); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateQuestions_MultipleQuestions(t *testing.T) {
	valid := map[string]any{
		"questions": []any{
			map[string]any{
				"id":       "q1",
				"question": "Q1",
				"options":  []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}},
			},
			map[string]any{
				"id":         "q2",
				"question":   "Q2",
				"options":    []any{map[string]any{"value": "x", "label": "X"}, map[string]any{"value": "y", "label": "Y"}},
				"allow_other": true,
			},
		},
	}
	if _, err := validateQuestions(valid); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateQuestions_MissingQuestions(t *testing.T) {
	if _, err := validateQuestions(map[string]any{}); err == nil {
		t.Fatal("expected error for missing questions")
	}
}

func TestValidateQuestions_TooMany(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{"id": "q1", "question": "Q1", "options": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}}},
			map[string]any{"id": "q2", "question": "Q2", "options": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}}},
			map[string]any{"id": "q3", "question": "Q3", "options": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}}},
			map[string]any{"id": "q4", "question": "Q4", "options": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}}},
		},
	}
	if _, err := validateQuestions(args); err == nil {
		t.Fatal("expected error for >3 questions")
	}
}

func TestValidateQuestions_DuplicateIDs(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{"id": "q1", "question": "Q1", "options": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}}},
			map[string]any{"id": "q1", "question": "Q2", "options": []any{map[string]any{"value": "x", "label": "X"}, map[string]any{"value": "y", "label": "Y"}}},
		},
	}
	if _, err := validateQuestions(args); err == nil {
		t.Fatal("expected error for duplicate question ids")
	}
}

func TestValidateQuestions_MissingOptionLabel(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{
				"id":       "q1",
				"question": "Q1",
				"options":  []any{map[string]any{"value": "a"}},
			},
		},
	}
	if _, err := validateQuestions(args); err == nil {
		t.Fatal("expected error for missing option label")
	}
}

func TestValidateQuestions_TooFewOptions(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{
				"id":       "q1",
				"question": "Q1",
				"options":  []any{map[string]any{"value": "a", "label": "A"}},
			},
		},
	}
	if _, err := validateQuestions(args); err == nil {
		t.Fatal("expected error for <2 options")
	}
}

func TestValidateQuestions_TooManyOptions(t *testing.T) {
	opts := make([]any, 7)
	for i := range opts {
		opts[i] = map[string]any{"value": string(rune('a' + i)), "label": string(rune('A' + i))}
	}
	args := map[string]any{
		"questions": []any{
			map[string]any{"id": "q1", "question": "Q1", "options": opts},
		},
	}
	if _, err := validateQuestions(args); err == nil {
		t.Fatal("expected error for >6 options")
	}
}

func TestValidateQuestions_MissingQuestionText(t *testing.T) {
	args := map[string]any{
		"questions": []any{
			map[string]any{
				"id":      "q1",
				"options": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "b", "label": "B"}},
			},
		},
	}
	if _, err := validateQuestions(args); err == nil {
		t.Fatal("expected error for missing question text")
	}
}
