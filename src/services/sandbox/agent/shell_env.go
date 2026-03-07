package main

import (
	"sort"
	"strings"
)

func ensureShellBaseDirs() error {
	return ensureWorkloadBaseDirs()
}

func buildShellEnv(snapshot map[string]string) []string {
	env := baseWorkloadEnv()
	env["TERM"] = "xterm-256color"
	env["HISTFILE"] = shellHomeDir + "/.bash_history"
	env["PS1"] = ""
	env["PROMPT_COMMAND"] = ""
	env["BASH_SILENCE_DEPRECATION_WARNING"] = "1"
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
