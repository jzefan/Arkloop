package executor

import (
"testing"

"arkloop/services/worker/internal/skills"
)

func TestClassifyRouteBuildsFromAutoSkillConfig(t *testing.T) {
root, err := skills.BuiltinSkillsRoot()
if err != nil {
t.Fatalf("BuiltinSkillsRoot failed: %v", err)
}
registry, err := skills.LoadRegistry(root)
if err != nil {
t.Fatalf("LoadRegistry failed: %v", err)
}
def, ok := registry.Get("auto")
if !ok {
t.Fatalf("expected auto skill loaded")
}

exec, err := NewClassifyRouteExecutor(def.ExecutorConfig)
if err != nil {
t.Fatalf("NewClassifyRouteExecutor failed: %v", err)
}
if exec == nil {
t.Fatalf("expected non-nil executor")
}
cre := exec.(*ClassifyRouteExecutor)
if cre.classifyPrompt == "" {
t.Fatalf("expected non-empty classify prompt")
}
if cre.defaultRoute != "pro" {
t.Fatalf("expected default_route 'pro', got %q", cre.defaultRoute)
}
if _, ok := cre.routes["pro"]; !ok {
t.Fatal("expected pro route")
}
if _, ok := cre.routes["ultra"]; !ok {
t.Fatal("expected ultra route")
}
}
