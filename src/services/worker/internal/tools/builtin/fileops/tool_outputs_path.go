package fileops

import (
	"fmt"
	"path/filepath"
	"strings"

	"arkloop/services/shared/objectstore"
)

const toolOutputsVirtualRoot = ".tool-outputs"
const toolOutputsObjectPrefix = "tool-outputs"

func toolOutputRelativePath(path string) (string, bool) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if cleaned == toolOutputsVirtualRoot {
		return "", true
	}
	prefix := toolOutputsVirtualRoot + "/"
	if strings.HasPrefix(cleaned, prefix) {
		return strings.TrimPrefix(cleaned, prefix), true
	}
	return "", false
}

func resolveScopedToolOutputObject(path string, scopeID string) (displayPath string, objectKey string, ok bool, err error) {
	rel, ok := toolOutputRelativePath(path)
	if !ok {
		return "", "", false, nil
	}
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return "", "", true, fmt.Errorf("tool output scope is not available")
	}
	cleanedRel := filepath.ToSlash(filepath.Clean(rel))
	if cleanedRel == "." || cleanedRel == "" {
		return "", "", true, fmt.Errorf("path %q is outside the current tool output scope", path)
	}
	if cleanedRel == scopeID {
		return filepath.ToSlash(filepath.Join(toolOutputsVirtualRoot, scopeID)), filepath.ToSlash(filepath.Join(toolOutputsObjectPrefix, scopeID)), true, nil
	}
	prefix := scopeID + "/"
	if !strings.HasPrefix(cleanedRel, prefix) {
		return "", "", true, fmt.Errorf("path %q is outside the current tool output scope", path)
	}
	return filepath.ToSlash(filepath.Join(toolOutputsVirtualRoot, cleanedRel)), filepath.ToSlash(filepath.Join(toolOutputsObjectPrefix, cleanedRel)), true, nil
}

func ResolveScopedToolOutputSearch(path string, scopeID string, store objectstore.Store) (objectPrefix, displayRoot string, ok bool, err error) {
	if store == nil {
		return "", "", false, nil
	}
	rel, ok := toolOutputRelativePath(path)
	if !ok {
		return "", "", false, nil
	}
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return "", "", true, fmt.Errorf("tool output scope is not available")
	}
	cleanedRel := filepath.ToSlash(filepath.Clean(rel))
	if cleanedRel == "." || cleanedRel == "" {
		return ToolOutputObjectPrefix(scopeID), filepath.ToSlash(filepath.Join(toolOutputsVirtualRoot, scopeID)), true, nil
	}
	displayPath, objectKey, ok, err := resolveScopedToolOutputObject(path, scopeID)
	if !ok || err != nil {
		return "", "", ok, err
	}
	_ = objectKey
	return objectKey, displayPath, true, nil
}

func ToolOutputDisplayPathFromObjectKey(objectKey string) (string, bool) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(objectKey)))
	prefix := toolOutputsObjectPrefix + "/"
	if !strings.HasPrefix(cleaned, prefix) {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(toolOutputsVirtualRoot, strings.TrimPrefix(cleaned, prefix))), true
}

func ToolOutputObjectPrefix(scopeID string) string {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return toolOutputsObjectPrefix + "/"
	}
	return filepath.ToSlash(filepath.Join(toolOutputsObjectPrefix, scopeID)) + "/"
}

func ThreadIDFromToolOutputObjectKey(objectKey string) string {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(objectKey)))
	prefix := toolOutputsObjectPrefix + "/"
	if !strings.HasPrefix(cleaned, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(cleaned, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}
