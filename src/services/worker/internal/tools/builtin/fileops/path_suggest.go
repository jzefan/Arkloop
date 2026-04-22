package fileops

import (
	"os"
	"path/filepath"
	"strings"
)

// SuggestSimilarPaths returns up to 3 similar path suggestions when a file is not found.
// Searches ±2 directory levels from the target path.
func SuggestSimilarPaths(targetPath, workDir string) []string {
	if targetPath == "" {
		return nil
	}

	var suggestions []string

	// strategy 1: same directory, different extension
	suggestions = append(suggestions, findSimilarInDir(targetPath)...)

	// strategy 2: check if the path exists relative to workDir (dropped repo folder)
	if workDir != "" {
		suggestions = append(suggestions, suggestUnderWorkDir(targetPath, workDir)...)
	}

	// dedup and cap at 3
	seen := make(map[string]struct{})
	unique := suggestions[:0]
	for _, s := range suggestions {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		unique = append(unique, s)
		if len(unique) >= 3 {
			break
		}
	}
	return unique
}

// findSimilarInDir finds files with same basename but different extension in the same directory.
func findSimilarInDir(targetPath string) []string {
	dir := filepath.Dir(targetPath)
	base := strings.TrimSuffix(filepath.Base(targetPath), filepath.Ext(targetPath))
	if base == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		entryBase := strings.TrimSuffix(name, filepath.Ext(name))
		if strings.EqualFold(entryBase, base) && name != filepath.Base(targetPath) {
			results = append(results, filepath.Join(dir, name))
		}
		if len(results) >= 3 {
			break
		}
	}
	return results
}

// suggestUnderWorkDir checks if a relative portion of the path exists under workDir.
// Handles the "dropped repo folder" pattern: /abs/path/repo/src/file.go -> workDir/src/file.go
func suggestUnderWorkDir(targetPath, workDir string) []string {
	// try progressively shorter suffixes of the target path
	parts := strings.Split(filepath.ToSlash(targetPath), "/")
	var results []string
	for i := 1; i < len(parts) && i < 4; i++ {
		suffix := filepath.Join(parts[i:]...)
		candidate := filepath.Join(workDir, suffix)
		if _, err := os.Stat(candidate); err == nil {
			results = append(results, candidate)
		}
	}
	return results
}
