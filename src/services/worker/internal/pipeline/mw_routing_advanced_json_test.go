package pipeline

import "testing"

func TestMergeAdvancedJSON_ModelOverridesProvider(t *testing.T) {
	merged := mergeAdvancedJSON(
		map[string]any{
			"metadata":    map[string]any{"source": "provider"},
			"temperature": 0.1,
		},
		map[string]any{
			"metadata": map[string]any{"source": "model"},
			"top_p":    0.9,
		},
	)

	metadata, ok := merged["metadata"].(map[string]any)
	if !ok || metadata["source"] != "model" {
		t.Fatalf("expected model metadata override, got %#v", merged)
	}
	if merged["temperature"] != 0.1 {
		t.Fatalf("expected provider key preserved, got %#v", merged)
	}
	if merged["top_p"] != 0.9 {
		t.Fatalf("expected model key merged, got %#v", merged)
	}
}

func TestMergeAdvancedJSON_EmptyInputs(t *testing.T) {
	merged := mergeAdvancedJSON(nil, nil)
	if len(merged) != 0 {
		t.Fatalf("expected empty map, got %#v", merged)
	}
}
