package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"arkloop/services/shared/skillstore"
)

const skillRootPath = "/opt/arkloop/skills"

func applySkillOverlay(req SkillOverlayRequest) error {
	if err := pruneEnvironmentRootChildren(shellSkillsDir); err != nil {
		return err
	}
	indexPath := filepath.Join(shellHomeDir, ".arkloop", "enabled-skills.json")
	if err := ensureEnvironmentRoot(filepath.Dir(indexPath)); err != nil {
		return err
	}
	if err := restoreRegularFile(strings.NewReader(req.IndexJSON), indexPath, 0o644); err != nil {
		return fmt.Errorf("write skill index: %w", err)
	}
	for _, item := range req.Skills {
		expected := skillstore.MountPath(strings.TrimSpace(item.SkillKey), strings.TrimSpace(item.Version))
		if strings.TrimSpace(item.MountPath) != expected {
			return fmt.Errorf("skill mount path is invalid")
		}
		mountRoot := filepath.Join(shellSkillsDir, strings.TrimSpace(item.SkillKey)+"@"+strings.TrimSpace(item.Version))
		if !pathWithinRoot(shellSkillsDir, mountRoot) {
			return fmt.Errorf("skill mount path escapes root")
		}
		encoded, err := base64.StdEncoding.DecodeString(item.BundleDataBase64)
		if err != nil {
			return fmt.Errorf("decode skill bundle: %w", err)
		}
		bundle, err := skillstore.DecodeBundle(encoded)
		if err != nil {
			return fmt.Errorf("decode skill archive: %w", err)
		}
		if bundle.Definition.SkillKey != strings.TrimSpace(item.SkillKey) || bundle.Definition.Version != strings.TrimSpace(item.Version) {
			return fmt.Errorf("skill definition mismatch")
		}
		if err := restoreSkillBundle(mountRoot, bundle); err != nil {
			return err
		}
	}
	return nil
}

func restoreSkillBundle(root string, bundle skillstore.BundleImage) error {
	if err := ensureEnvironmentRoot(root); err != nil {
		return err
	}
	for _, file := range bundle.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if !pathWithinRoot(root, target) {
			return fmt.Errorf("skill file escapes root: %s", file.Path)
		}
		if len(file.Data) == 0 {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create skill dir %s: %w", target, err)
			}
			if err := os.Chmod(target, readonlyDirMode(file.Mode)); err != nil {
				return fmt.Errorf("chmod skill dir %s: %w", target, err)
			}
			continue
		}
		if err := restoreRegularFile(bytes.NewReader(file.Data), target, readonlyFileMode(file.Mode)); err != nil {
			return err
		}
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, readonlyDirMode(int64(info.Mode().Perm())))
		}
		return os.Chmod(path, os.FileMode(readonlyFileMode(int64(info.Mode().Perm()))))
	})
}

func readonlyFileMode(mode int64) int64 {
	if mode&0o111 != 0 {
		return 0o555
	}
	return 0o444
}

func readonlyDirMode(_ int64) os.FileMode {
	return 0o555
}
