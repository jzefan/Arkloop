package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	workloadUID          = 1000
	workloadGID          = 1000
	workloadUser         = "arkloop"
	python3Bin           = "/usr/local/bin/python3"
	chartPreludePath     = "/usr/local/share/arkloop/chart_prelude.py"
	chartPreludeStmt     = "try:\n exec(open('" + chartPreludePath + "').read())\nexcept FileNotFoundError:\n pass\n"
	defaultWorkloadCwd   = "/workspace"
	defaultWorkloadHome  = "/home/arkloop"
	defaultSkillsRoot    = "/opt/arkloop/skills"
	defaultWorkloadTmp   = "/tmp/arkloop"
	defaultWorkloadPath  = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	defaultWorkloadLang  = "C.UTF-8"
	matplotlibConfigDir  = "/tmp/matplotlib"
)

var shellWorkspaceDir = defaultWorkloadCwd
var shellHomeDir = defaultWorkloadHome
var shellSkillsDir = defaultSkillsRoot
var shellTempDir = defaultWorkloadTmp

func ensureWorkloadBaseDirs() error {
	for _, dir := range []string{shellWorkspaceDir, shellHomeDir, shellSkillsDir, shellTempDir, artifactOutputDir, matplotlibConfigDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create workload dir %s: %w", dir, err)
		}
		if err := chownIfPossible(dir); err != nil {
			return err
		}
	}
	return nil
}

func baseWorkloadEnv() map[string]string {
	return map[string]string{
		"HOME":         shellHomeDir,
		"PATH":         defaultWorkloadPath,
		"LANG":         defaultWorkloadLang,
		"TMPDIR":       shellTempDir,
		"MPLCONFIGDIR": matplotlibConfigDir,
		"USER":         workloadUser,
		"LOGNAME":      workloadUser,
	}
}

func buildWorkloadEnv(overrides map[string]string) []string {
	env := baseWorkloadEnv()
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsRune(key, '=') {
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

func prepareWorkloadCmd(cmd *exec.Cmd, cwd string, extraEnv map[string]string) {
	if strings.TrimSpace(cwd) == "" {
		cwd = shellWorkspaceDir
	}
	cmd.Dir = cwd
	cmd.Env = buildWorkloadEnv(extraEnv)
	if os.Geteuid() == 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: workloadUID,
				Gid: workloadGID,
			},
		}
	}
}

func chownIfPossible(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	if err := os.Chown(path, workloadUID, workloadGID); err != nil {
		return fmt.Errorf("chown %s: %w", path, err)
	}
	return nil
}

func chownTreeIfPossible(root string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return filepath.Walk(root, func(current string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Lchown(current, workloadUID, workloadGID); err != nil {
			return fmt.Errorf("chown %s: %w", current, err)
		}
		return nil
	})
}
