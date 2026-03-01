package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRegistryLoadsLiteSkill(t *testing.T) {
	root, err := BuiltinSkillsRoot()
	if err != nil {
		t.Fatalf("BuiltinSkillsRoot failed: %v", err)
	}
	registry, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("lite")
	if !ok {
		t.Fatalf("expected lite skill loaded")
	}
	if def.Version != "1" {
		t.Fatalf("unexpected version: %s", def.Version)
	}
	if def.PromptMD == "" {
		t.Fatalf("expected prompt md")
	}
}

func TestResolveSkillVersionMismatch(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{ID: "demo", Version: "1", Title: "t"}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	decision := ResolveSkill(map[string]any{"skill_id": "demo@2"}, registry)
	if decision.Error == nil || decision.Error.ErrorClass != ErrorClassSkillVersionMismatch {
		t.Fatalf("expected version mismatch, got %+v", decision)
	}
}

// TestLoadSkillDefaultExecutorType 验证无 executor_type 字段的 yaml 使用默认值，向后兼容。
func TestLoadSkillDefaultExecutorType(t *testing.T) {
	root, err := BuiltinSkillsRoot()
	if err != nil {
		t.Fatalf("BuiltinSkillsRoot failed: %v", err)
	}
	registry, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("lite")
	if !ok {
		t.Fatalf("expected lite skill loaded")
	}
	if def.ExecutorType != "agent.simple" {
		t.Fatalf("expected default executor_type 'agent.simple', got %q", def.ExecutorType)
	}
	if def.ExecutorConfig == nil {
		t.Fatalf("expected non-nil ExecutorConfig")
	}
}

// TestLoadSkillWithExecutorType 验证 executor_type 和 executor_config 字段可正确解析。
func TestLoadSkillWithExecutorType(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "test_exec",
		"id: test_exec\nversion: \"1\"\ntitle: Test\nexecutor_type: task.classify_route\nexecutor_config:\n  check_in_every: 5\n",
		"# prompt",
	)

	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("test_exec")
	if !ok {
		t.Fatalf("expected test_exec skill loaded")
	}
	if def.ExecutorType != "task.classify_route" {
		t.Fatalf("expected executor_type 'task.classify_route', got %q", def.ExecutorType)
	}
	if def.ExecutorConfig["check_in_every"] == nil {
		t.Fatalf("expected executor_config.check_in_every to be set")
	}
}

func TestLoadSkillWithExecutorScriptFile(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "test_lua",
		"id: test_lua\nversion: \"1\"\ntitle: Test Lua\nexecutor_type: agent.lua\nexecutor_config:\n  script_file: agent.lua\n",
		"# prompt",
	)
	if err := os.WriteFile(filepath.Join(dir, "test_lua", "agent.lua"), []byte("context.set_output('ok')\n"), 0644); err != nil {
		t.Fatalf("WriteFile agent.lua failed: %v", err)
	}

	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("test_lua")
	if !ok {
		t.Fatalf("expected test_lua skill loaded")
	}
	script, ok := def.ExecutorConfig["script"].(string)
	if !ok || script == "" {
		t.Fatalf("expected executor_config.script from script_file")
	}
	if _, exists := def.ExecutorConfig["script_file"]; exists {
		t.Fatalf("expected script_file removed after loading")
	}
}

func TestLoadSkillWithExecutorScriptFileConflict(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "bad_lua",
		"id: bad_lua\nversion: \"1\"\ntitle: Bad Lua\nexecutor_type: agent.lua\nexecutor_config:\n  script: |\n    context.set_output('inline')\n  script_file: agent.lua\n",
		"# prompt",
	)
	if err := os.WriteFile(filepath.Join(dir, "bad_lua", "agent.lua"), []byte("context.set_output('file')\n"), 0644); err != nil {
		t.Fatalf("WriteFile agent.lua failed: %v", err)
	}

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for script and script_file conflict, got nil")
	}
}

func TestLoadSkillWithExecutorScriptFileEscape(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "escape_lua",
		"id: escape_lua\nversion: \"1\"\ntitle: Escape Lua\nexecutor_type: agent.lua\nexecutor_config:\n  script_file: ../agent.lua\n",
		"# prompt",
	)

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for escaped script_file path, got nil")
	}
}

func TestLoadSkillWithNonLuaExecutorKeepsScriptFileRaw(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "route_keep_raw",
		"id: route_keep_raw\nversion: \"1\"\ntitle: Route Keep Raw\nexecutor_type: task.classify_route\nexecutor_config:\n  script_file: untouched.lua\n",
		"# prompt",
	)

	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("route_keep_raw")
	if !ok {
		t.Fatalf("expected route_keep_raw skill loaded")
	}
	raw, ok := def.ExecutorConfig["script_file"].(string)
	if !ok || raw != "untouched.lua" {
		t.Fatalf("expected script_file untouched for non-lua executor, got %#v", def.ExecutorConfig["script_file"])
	}
}

// TestLoadSkillInvalidExecutorType 验证非法 executor_type 返回错误。
func TestLoadSkillInvalidExecutorType(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "bad_exec",
		"id: bad_exec\nversion: \"1\"\ntitle: Bad\nexecutor_type: \"!!invalid type!!\"\n",
		"# prompt",
	)

	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("expected error for invalid executor_type, got nil")
	}
}

func writeSkillFiles(t *testing.T, root, name, yamlContent, promptContent string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skill.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("WriteFile skill.yaml failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(promptContent), 0644); err != nil {
		t.Fatalf("WriteFile prompt.md failed: %v", err)
	}
}

// TestLoadSkillWithPreferredCredential 验证 preferred_credential 字段可正确解析。
func TestLoadSkillWithPreferredCredential(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "test_route",
		"id: test_route\nversion: \"1\"\ntitle: Test\npreferred_credential: my-anthropic\n",
		"# prompt",
	)

	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("test_route")
	if !ok {
		t.Fatalf("expected test_route skill loaded")
	}
	if def.PreferredCredential == nil {
		t.Fatal("expected PreferredCredential to be set")
	}
	if *def.PreferredCredential != "my-anthropic" {
		t.Fatalf("expected preferred_credential 'my-anthropic', got %q", *def.PreferredCredential)
	}
}

// TestLoadSkillWithoutPreferredCredential 验证无 preferred_credential 字段时 PreferredCredential 为 nil。
func TestLoadSkillWithoutPreferredCredential(t *testing.T) {
	dir := t.TempDir()
	writeSkillFiles(t, dir, "no_cred_skill",
		"id: no_cred_skill\nversion: \"1\"\ntitle: No Cred\n",
		"# prompt",
	)

	registry, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry failed: %v", err)
	}
	def, ok := registry.Get("no_cred_skill")
	if !ok {
		t.Fatal("expected no_cred_skill to be loaded")
	}
	if def.PreferredCredential != nil {
		t.Fatalf("expected PreferredCredential nil, got %q", *def.PreferredCredential)
	}
}

func TestMergeRegistryKeepsBaseTitleSummarizerWhenOverrideMissing(t *testing.T) {
	base := NewRegistry()
	if err := base.Register(Definition{
		ID:      "pro",
		Version: "1",
		Title:   "Pro",
		TitleSummarizer: &TitleSummarizerConfig{
			Prompt:    "base prompt",
			MaxTokens: 15,
		},
	}); err != nil {
		t.Fatalf("register base failed: %v", err)
	}

	merged := MergeRegistry(base, []Definition{
		{
			ID:      "pro",
			Version: "1",
			Title:   "Pro Override",
		},
	})

	def, ok := merged.Get("pro")
	if !ok {
		t.Fatal("expected merged registry has pro")
	}
	if def.TitleSummarizer == nil {
		t.Fatal("expected title summarizer preserved from base")
	}
	if def.TitleSummarizer.Prompt != "base prompt" {
		t.Fatalf("unexpected prompt: %q", def.TitleSummarizer.Prompt)
	}
	if def.Title != "Pro Override" {
		t.Fatalf("expected override title, got %q", def.Title)
	}
}
