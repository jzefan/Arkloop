package personas

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

func BuiltinPersonasRoot() (string, error) {
	if envRoot := os.Getenv("ARKLOOP_PERSONAS_ROOT"); envRoot != "" {
		return envRoot, nil
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate personas root directory")
	}
	dir := filepath.Dir(filename)
	for {
		if filepath.Base(dir) == "src" {
			return filepath.Join(dir, "personas"), nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "", fmt.Errorf("src directory not found, cannot locate personas root directory")
}

type RepoPersona struct {
	ID                  string         `yaml:"id"`
	Version             string         `yaml:"version"`
	Title               string         `yaml:"title"`
	Description         string         `yaml:"description"`
	UserSelectable      bool           `yaml:"user_selectable"`
	SelectorName        string         `yaml:"selector_name"`
	SelectorOrder       *int           `yaml:"selector_order"`
	ToolAllowlist       []string       `yaml:"tool_allowlist"`
	ToolDenylist        []string       `yaml:"tool_denylist"`
	Budgets             map[string]any `yaml:"budgets"`
	PreferredCredential string         `yaml:"preferred_credential"`
	Model               string         `yaml:"model"`
	ReasoningMode       string         `yaml:"reasoning_mode"`
	PromptCacheControl  string         `yaml:"prompt_cache_control"`
	ExecutorType        string         `yaml:"executor_type"`
	ExecutorConfig      map[string]any `yaml:"executor_config"`
	PromptMD            string         `yaml:"-"`
}

func LoadFromDir(root string) ([]RepoPersona, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []RepoPersona
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		dir := filepath.Join(root, entry.Name())
		yamlPath := filepath.Join(dir, "persona.yaml")
		promptPath := filepath.Join(dir, "prompt.md")

		yamlData, err := os.ReadFile(yamlPath)
		if err != nil {
			continue
		}

		var p RepoPersona
		if err := yaml.Unmarshal(yamlData, &p); err != nil {
			continue
		}
		if p.ID == "" {
			continue
		}
		if p.Version == "" {
			p.Version = "1"
		}
		if promptData, err := os.ReadFile(promptPath); err == nil {
			p.PromptMD = string(promptData)
		}

		result = append(result, p)
	}
	return result, nil
}
