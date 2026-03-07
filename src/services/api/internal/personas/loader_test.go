package personas

import "testing"

func TestBuiltinPersonasRootLoadsRepoPersonas(t *testing.T) {
	root, err := BuiltinPersonasRoot()
	if err != nil {
		t.Fatalf("BuiltinPersonasRoot failed: %v", err)
	}
	personas, err := LoadFromDir(root)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}
	if len(personas) == 0 {
		t.Fatal("expected repo personas loaded")
	}

	seen := map[string]RepoPersona{}
	for _, persona := range personas {
		seen[persona.ID] = persona
	}
	if _, ok := seen["normal"]; !ok {
		t.Fatal("expected normal persona loaded")
	}
	if _, ok := seen["extended-search"]; !ok {
		t.Fatal("expected extended-search persona loaded")
	}
}
