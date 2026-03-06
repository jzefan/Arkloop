package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func ensureShellBaseDirs() error {
	for _, dir := range []string{shellWorkspaceDir, shellHomeDir, shellTempDir, artifactOutputDir, "/tmp/matplotlib"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create shell dir %s: %w", dir, err)
		}
	}
	return nil
}

func buildShellEnv(snapshot map[string]string) []string {
	env := map[string]string{
		"HOME":                             shellHomeDir,
		"PATH":                             defaultShellPath,
		"LANG":                             defaultShellLang,
		"TERM":                             "xterm-256color",
		"TMPDIR":                           shellTempDir,
		"MPLCONFIGDIR":                     "/tmp/matplotlib",
		"HISTFILE":                         shellHomeDir + "/.bash_history",
		"USER":                             "arkloop",
		"LOGNAME":                          "arkloop",
		"PS1":                              "",
		"PROMPT_COMMAND":                   "",
		"BASH_SILENCE_DEPRECATION_WARNING": "1",
	}
	for key, value := range snapshot {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsRune(key, '=') {
			continue
		}
		switch key {
		case "HOME", "PATH", "PWD", "OLDPWD", "SHLVL", "_", "PS1", "PROMPT_COMMAND", "BASH_SILENCE_DEPRECATION_WARNING":
			continue
		}
		env[key] = value
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result
}
