//go:build desktop

package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
}

type ServerConfig struct {
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Extensions  []string          `json:"extensions"`
	InitOptions json.RawMessage   `json:"initOptions,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// LoadConfig reads LSP configuration from ~/.arkloop/lsp.json.
// Returns an empty config (not an error) if the file doesn't exist.
func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}

	path := filepath.Join(home, ".arkloop", "lsp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Servers: map[string]ServerConfig{}}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Servers == nil {
		cfg.Servers = map[string]ServerConfig{}
	}

	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks config integrity: required fields, extension format,
// and no duplicate extensions across servers.
func Validate(cfg *Config) error {
	extOwner := make(map[string]string) // ext -> server name

	for name, sc := range cfg.Servers {
		if strings.TrimSpace(sc.Command) == "" {
			return fmt.Errorf("server %q: command is required", name)
		}
		if len(sc.Extensions) == 0 {
			return fmt.Errorf("server %q: at least one extension is required", name)
		}
		for _, ext := range sc.Extensions {
			if !strings.HasPrefix(ext, ".") {
				return fmt.Errorf("server %q: extension %q must start with '.'", name, ext)
			}
			lower := strings.ToLower(ext)
			if prev, ok := extOwner[lower]; ok {
				return fmt.Errorf("duplicate extension %q: claimed by both %q and %q", ext, prev, name)
			}
			extOwner[lower] = name
		}

		if _, err := exec.LookPath(sc.Command); err != nil {
			slog.Warn("lsp server command not found in PATH", "server", name, "command", sc.Command)
		}
	}
	return nil
}

func (c *Config) IsEmpty() bool {
	return len(c.Servers) == 0
}

// ServerForExtension returns the server name and config for a given file extension.
// ext should be lowercase with a leading dot (e.g. ".go").
func (c *Config) ServerForExtension(ext string) (string, *ServerConfig, bool) {
	lower := strings.ToLower(ext)
	for name, sc := range c.Servers {
		for _, e := range sc.Extensions {
			if strings.ToLower(e) == lower {
				return name, &sc, true
			}
		}
	}
	return "", nil, false
}
