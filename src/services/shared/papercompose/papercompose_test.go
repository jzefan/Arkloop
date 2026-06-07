package papercompose

import (
	"reflect"
	"testing"
)

func selectedIDs(qs []Question) []string {
	out := make([]string, 0, len(qs))
	for _, q := range qs {
		out = append(out, q.ID)
	}
	return out
}

func TestComposeHonorsDifficultyDistribution(t *testing.T) {
	pool := []Question{
		{ID: "1", Type: "single_choice", Difficulty: "easy"},
		{ID: "2", Type: "single_choice", Difficulty: "medium"},
		{ID: "3", Type: "single_choice", Difficulty: "medium"},
		{ID: "4", Type: "single_choice", Difficulty: "hard"},
	}
	sel, sh := Compose(pool, Spec{Total: 3, DifficultyDist: map[string]int{"easy": 1, "medium": 2}}, 0)
	if len(sh) != 0 {
		t.Fatalf("expected no shortages, got %+v", sh)
	}
	if len(sel) != 3 {
		t.Fatalf("expected 3 selected, got %d", len(sel))
	}
	counts := map[string]int{}
	for _, q := range sel {
		counts[q.Difficulty]++
	}
	if counts["easy"] != 1 || counts["medium"] != 2 {
		t.Fatalf("bad difficulty distribution: %+v", counts)
	}
}

func TestComposeHonorsTypeDistribution(t *testing.T) {
	pool := []Question{
		{ID: "1", Type: "single_choice", Difficulty: "easy"},
		{ID: "2", Type: "single_choice", Difficulty: "easy"},
		{ID: "3", Type: "fill_in", Difficulty: "easy"},
		{ID: "4", Type: "fill_in", Difficulty: "easy"},
	}
	sel, sh := Compose(pool, Spec{Total: 3, TypeDist: map[string]int{"single_choice": 2, "fill_in": 1}}, 0)
	if len(sh) != 0 {
		t.Fatalf("expected no shortages, got %+v", sh)
	}
	counts := map[string]int{}
	for _, q := range sel {
		counts[q.Type]++
	}
	if counts["single_choice"] != 2 || counts["fill_in"] != 1 {
		t.Fatalf("bad type distribution: %+v", counts)
	}
}

func TestComposeReportsTypeShortage(t *testing.T) {
	sel, sh := Compose([]Question{{ID: "1", Type: "single_choice"}},
		Spec{Total: 2, TypeDist: map[string]int{"single_choice": 2}}, 0)
	if len(sel) != 0 {
		t.Fatalf("expected no selected, got %d", len(sel))
	}
	if len(sh) != 1 || sh[0].Dimension != "type" || sh[0].Value != "single_choice" {
		t.Fatalf("expected type/single_choice shortage, got %+v", sh)
	}
	if sh[0].Requested != 2 || sh[0].Available != 1 {
		t.Fatalf("expected requested=2 available=1, got %+v", sh[0])
	}
}

func TestComposeReportsKnowledgePointShortage(t *testing.T) {
	sel, sh := Compose([]Question{
		{ID: "1", KnowledgePointID: "kp1", Type: "single_choice", Difficulty: "medium"},
		{ID: "2", KnowledgePointID: "kp2", Type: "single_choice", Difficulty: "medium"},
	}, Spec{Total: 2, KPDist: map[string]int{"kp1": 2}}, 0)
	if len(sel) != 0 {
		t.Fatalf("expected no selected, got %d", len(sel))
	}
	if len(sh) != 1 || sh[0].Dimension != "knowledge_point_id" || sh[0].Value != "kp1" {
		t.Fatalf("expected knowledge_point_id/kp1 shortage, got %+v", sh)
	}
}

func TestComposeReportsTotalShortage(t *testing.T) {
	sel, sh := Compose([]Question{{ID: "1"}, {ID: "2"}}, Spec{Total: 5}, 0)
	if len(sel) != 0 {
		t.Fatalf("expected no selected, got %d", len(sel))
	}
	if len(sh) != 1 || sh[0].Dimension != "" || sh[0].Requested != 5 || sh[0].Available != 2 {
		t.Fatalf("expected total shortage requested=5 available=2, got %+v", sh)
	}
}

func TestComposeReportsOverConstrained(t *testing.T) {
	pool := []Question{
		{ID: "1", Type: "single_choice"},
		{ID: "2", Type: "single_choice"},
		{ID: "3", Type: "single_choice"},
	}
	// requested type sum (3) exceeds total (2)
	sel, sh := Compose(pool, Spec{Total: 2, TypeDist: map[string]int{"single_choice": 3}}, 0)
	if len(sel) != 0 {
		t.Fatalf("expected no selected, got %d", len(sel))
	}
	if len(sh) == 0 || sh[0].Value != "all" {
		t.Fatalf("expected over-constrained 'all' shortage, got %+v", sh)
	}
}

func TestComposeSeedDeterministic(t *testing.T) {
	pool := []Question{
		{ID: "1", Type: "single_choice", Difficulty: "easy"},
		{ID: "2", Type: "single_choice", Difficulty: "easy"},
		{ID: "3", Type: "single_choice", Difficulty: "easy"},
		{ID: "4", Type: "single_choice", Difficulty: "easy"},
		{ID: "5", Type: "single_choice", Difficulty: "easy"},
	}
	a, _ := Compose(pool, Spec{Total: 3}, 42)
	b, _ := Compose(pool, Spec{Total: 3}, 42)
	if !reflect.DeepEqual(selectedIDs(a), selectedIDs(b)) {
		t.Fatalf("same seed must be deterministic: %v vs %v", selectedIDs(a), selectedIDs(b))
	}
}

func TestComposeDoesNotMutateInput(t *testing.T) {
	pool := []Question{
		{ID: "3"}, {ID: "1"}, {ID: "2"},
	}
	before := selectedIDs(pool)
	Compose(pool, Spec{Total: 2}, 99)
	after := selectedIDs(pool)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("Compose must not mutate caller's pool: before=%v after=%v", before, after)
	}
}

func TestComposeNoConstraintsTakesTotal(t *testing.T) {
	pool := []Question{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	sel, sh := Compose(pool, Spec{Total: 2}, 0)
	if len(sh) != 0 {
		t.Fatalf("expected no shortages, got %+v", sh)
	}
	if len(sel) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(sel))
	}
}
