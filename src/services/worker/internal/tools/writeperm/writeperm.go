//go:build desktop

package writeperm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	envPermissionURL = "ARKLOOP_WRITE_PERMISSION_URL"
	envDesktopToken  = "ARKLOOP_DESKTOP_TOKEN"
)

type request struct {
	WorkspaceRoot string `json:"workspace_root"`
	Path          string `json:"path"`
	Action        string `json:"action"`
}

type response struct {
	Allowed bool   `json:"allowed"`
	Scope   string `json:"scope"`
	Error   string `json:"error"`
}

func Check(ctx context.Context, workspaceRoot string, targetPath string, action string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	targetPath = strings.TrimSpace(targetPath)
	if workspaceRoot == "" || targetPath == "" {
		return fmt.Errorf("workspace root and path are required")
	}
	if !isInsideWorkspace(workspaceRoot, targetPath) {
		return fmt.Errorf("path %q is outside the workspace", targetPath)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(envPermissionURL)), "/")
	if baseURL == "" {
		return nil
	}

	payload, err := json.Marshal(request{
		WorkspaceRoot: workspaceRoot,
		Path:          targetPath,
		Action:        strings.TrimSpace(action),
	})
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/v1/write-permission", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(os.Getenv(envDesktopToken)); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("write permission request failed: %w", err)
	}
	defer resp.Body.Close()
	var decoded response
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return fmt.Errorf("write permission response invalid: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if decoded.Error != "" {
			return fmt.Errorf("write permission denied: %s", decoded.Error)
		}
		return fmt.Errorf("write permission denied")
	}
	if !decoded.Allowed {
		return fmt.Errorf("write permission denied")
	}
	return nil
}

func isInsideWorkspace(workspaceRoot string, targetPath string) bool {
	root := filepath.Clean(workspaceRoot)
	resolved := targetPath
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, resolved)
	}
	resolved = filepath.Clean(resolved)
	return resolved == root || strings.HasPrefix(resolved, root+string(filepath.Separator))
}
