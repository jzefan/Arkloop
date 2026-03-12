package askuser

import (
	"testing"
)

func TestValidateAndNormalize_EnumField(t *testing.T) {
	args := map[string]any{
		"message": "Choose a database",
		"fields": []any{
			map[string]any{"key": "db", "type": "string", "title": "Database", "enum": []any{"postgres", "mysql", "sqlite"}, "required": true},
		},
	}
	msg, schema, err := ValidateAndNormalize(args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	if msg != "Choose a database" {
		t.Fatalf("message mismatch: %v", msg)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["db"]; !ok {
		t.Fatal("expected db property")
	}
	if req, ok := schema["required"].([]any); !ok || len(req) != 1 || req[0] != "db" {
		t.Fatalf("expected required=[db], got: %v", schema["required"])
	}
}

func TestValidateAndNormalize_OneOfField(t *testing.T) {
	args := map[string]any{
		"message": "Choose approach",
		"fields": []any{
			map[string]any{
				"key": "approach", "type": "string", "title": "Approach",
				"oneOf": []any{
					map[string]any{"const": "a", "title": "Approach A"},
					map[string]any{"const": "b", "title": "Approach B"},
				},
			},
		},
	}
	_, schema, err := ValidateAndNormalize(args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	props := schema["properties"].(map[string]any)
	f := props["approach"].(map[string]any)
	if _, ok := f["oneOf"]; !ok {
		t.Fatal("expected oneOf in property")
	}
}

func TestValidateAndNormalize_MultiselectEnum(t *testing.T) {
	args := map[string]any{
		"message": "Select features",
		"fields": []any{
			map[string]any{"key": "features", "type": "array", "title": "Features", "enum": []any{"auth", "billing", "search"}, "required": true},
		},
	}
	_, schema, err := ValidateAndNormalize(args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	props := schema["properties"].(map[string]any)
	f := props["features"].(map[string]any)
	items, ok := f["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items in array property")
	}
	if _, ok := items["enum"]; !ok {
		t.Fatal("expected enum in items")
	}
}

func TestValidateAndNormalize_MultiselectOneOf(t *testing.T) {
	args := map[string]any{
		"message": "Select features",
		"fields": []any{
			map[string]any{
				"key": "features", "type": "array", "title": "Features",
				"oneOf": []any{
					map[string]any{"const": "auth", "title": "Authentication"},
					map[string]any{"const": "billing", "title": "Billing"},
				},
			},
		},
	}
	_, schema, err := ValidateAndNormalize(args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	props := schema["properties"].(map[string]any)
	f := props["features"].(map[string]any)
	items, ok := f["items"].(map[string]any)
	if !ok {
		t.Fatal("expected items in array property")
	}
	if _, ok := items["anyOf"]; !ok {
		t.Fatal("expected anyOf in items")
	}
}

func TestValidateAndNormalize_BooleanField(t *testing.T) {
	args := map[string]any{
		"message": "Enable caching?",
		"fields": []any{
			map[string]any{"key": "cache", "type": "boolean", "title": "Enable cache", "default": true},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateAndNormalize_TextField(t *testing.T) {
	args := map[string]any{
		"message": "Enter project name",
		"fields": []any{
			map[string]any{"key": "name", "type": "string", "title": "Project name", "required": true},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateAndNormalize_NumberField(t *testing.T) {
	args := map[string]any{
		"message": "Choose port",
		"fields": []any{
			map[string]any{"key": "port", "type": "integer", "title": "Port", "default": float64(8080)},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateAndNormalize_MultipleFields(t *testing.T) {
	args := map[string]any{
		"message": "Configure project",
		"fields": []any{
			map[string]any{"key": "db", "type": "string", "title": "Database", "enum": []any{"postgres", "mysql"}, "required": true},
			map[string]any{"key": "cache", "type": "boolean", "title": "Enable cache"},
			map[string]any{"key": "name", "type": "string", "title": "Name", "required": true},
			map[string]any{"key": "workers", "type": "number", "title": "Workers", "minimum": float64(1)},
		},
	}
	_, schema, err := ValidateAndNormalize(args)
	if err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
	props := schema["properties"].(map[string]any)
	if len(props) != 4 {
		t.Fatalf("expected 4 properties, got %d", len(props))
	}
	req := schema["required"].([]any)
	if len(req) != 2 {
		t.Fatalf("expected 2 required, got %d", len(req))
	}
}

func TestValidateAndNormalize_MissingMessage(t *testing.T) {
	args := map[string]any{
		"fields": []any{
			map[string]any{"key": "db", "type": "string", "enum": []any{"a", "b"}},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestValidateAndNormalize_MissingFields(t *testing.T) {
	args := map[string]any{
		"message": "hello",
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestValidateAndNormalize_EmptyFields(t *testing.T) {
	args := map[string]any{
		"message": "hello",
		"fields":  []any{},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for empty fields")
	}
}

func TestValidateAndNormalize_DuplicateKey(t *testing.T) {
	args := map[string]any{
		"message": "test",
		"fields": []any{
			map[string]any{"key": "x", "type": "string"},
			map[string]any{"key": "x", "type": "boolean"},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for duplicate key")
	}
}

func TestValidateAndNormalize_UnsupportedType(t *testing.T) {
	args := map[string]any{
		"message": "test",
		"fields": []any{
			map[string]any{"key": "x", "type": "object"},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestValidateAndNormalize_EmptyEnum(t *testing.T) {
	args := map[string]any{
		"message": "test",
		"fields": []any{
			map[string]any{"key": "x", "type": "string", "enum": []any{}},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for empty enum")
	}
}

func TestValidateAndNormalize_OneOfMissingConst(t *testing.T) {
	args := map[string]any{
		"message": "test",
		"fields": []any{
			map[string]any{
				"key": "x", "type": "string",
				"oneOf": []any{map[string]any{"title": "A"}},
			},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for oneOf missing const")
	}
}

func TestValidateAndNormalize_ArrayMissingOptions(t *testing.T) {
	args := map[string]any{
		"message": "test",
		"fields": []any{
			map[string]any{"key": "x", "type": "array"},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for array missing enum/oneOf")
	}
}

func TestValidateAndNormalize_MissingKey(t *testing.T) {
	args := map[string]any{
		"message": "test",
		"fields": []any{
			map[string]any{"type": "string"},
		},
	}
	if _, _, err := ValidateAndNormalize(args); err == nil {
		t.Fatal("expected error for missing key")
	}
}
