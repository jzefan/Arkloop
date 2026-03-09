package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func pathWithinRoot(rootPath, targetPath string) bool {
	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func linkTargetWithinRoot(rootPath, currentPath, linkTarget string) bool {
	resolved := linkTarget
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(currentPath), resolved)
	}
	return pathWithinRoot(rootPath, resolved)
}

func fileModeOrDefault(mode int64, fallback os.FileMode) os.FileMode {
	if mode <= 0 {
		return fallback
	}
	return os.FileMode(mode)
}

func restoreRegularFile(reader io.Reader, targetPath string, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("restore file parent %s: %w", targetPath, err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), ".restore-*")
	if err != nil {
		return fmt.Errorf("create restore temp file %s: %w", targetPath, err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := io.Copy(tempFile, reader); err != nil {
		tempFile.Close()
		return fmt.Errorf("write restore file %s: %w", targetPath, err)
	}
	if err := tempFile.Chmod(fileModeOrDefault(mode, 0o644)); err != nil {
		tempFile.Close()
		return fmt.Errorf("chmod restore file %s: %w", targetPath, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close restore file %s: %w", targetPath, err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("replace restore file %s: %w", targetPath, err)
	}
	return nil
}

func resetEnvironmentRoot(rootPath string) error {
	if err := os.RemoveAll(rootPath); err != nil {
		return err
	}
	return os.MkdirAll(rootPath, 0o755)
}
