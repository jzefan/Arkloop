package fileops

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
)

// LocalBackend performs file operations directly on the host filesystem,
// rooted under WorkDir. All paths are resolved relative to WorkDir.
type LocalBackend struct {
	WorkDir           string
	ToolOutputScopeID string
	ToolOutputStore   objectstore.Store
}

func (b *LocalBackend) resolvePath(path string) (string, error) {
	return resolvePathWithinRoot(b.WorkDir, path)
}

func resolvePathWithinRoot(root string, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	cleaned := filepath.Clean(path)
	wsClean := filepath.Clean(root)
	if !strings.HasPrefix(cleaned, wsClean+string(filepath.Separator)) && cleaned != wsClean {
		return "", fmt.Errorf("path %q is outside the workspace (path traversal blocked)", path)
	}
	return cleaned, nil
}

func (b *LocalBackend) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if _, objectKey, ok, err := resolveScopedToolOutputObject(path, b.ToolOutputScopeID); ok {
		if err != nil {
			return nil, err
		}
		if b.ToolOutputStore == nil {
			return nil, os.ErrNotExist
		}
		return b.ToolOutputStore.Get(ctx, objectKey)
	}
	resolved, err := b.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

func (b *LocalBackend) NormalizePath(path string) string {
	if displayPath, _, ok, err := resolveScopedToolOutputObject(path, b.ToolOutputScopeID); ok {
		if err != nil {
			return normalizePathKey(path)
		}
		return displayPath
	}
	resolved, err := b.resolvePath(path)
	if err != nil {
		return normalizePathKey(path)
	}
	return filepath.ToSlash(resolved)
}

func (b *LocalBackend) WriteFile(ctx context.Context, path string, data []byte) error {
	if _, objectKey, ok, err := resolveScopedToolOutputObject(path, b.ToolOutputScopeID); ok {
		if err != nil {
			return err
		}
		if b.ToolOutputStore == nil {
			return fmt.Errorf("tool output store is unavailable")
		}
		return b.ToolOutputStore.PutObject(ctx, objectKey, data, objectstore.PutOptions{
			ContentType: "text/plain; charset=utf-8",
			Metadata: map[string]string{
				"scope_id":   strings.TrimSpace(b.ToolOutputScopeID),
				"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
			},
		})
	}
	resolved, err := b.resolvePath(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directories: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".arkloop-write-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, resolved); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (b *LocalBackend) Stat(ctx context.Context, path string) (FileInfo, error) {
	if _, objectKey, ok, err := resolveScopedToolOutputObject(path, b.ToolOutputScopeID); ok {
		if err != nil {
			return FileInfo{}, err
		}
		if b.ToolOutputStore == nil {
			return FileInfo{}, os.ErrNotExist
		}
		info, headErr := b.ToolOutputStore.Head(ctx, objectKey)
		if headErr != nil {
			return FileInfo{}, headErr
		}
		modTime := time.Time{}
		if raw := strings.TrimSpace(info.Metadata["updated_at"]); raw != "" {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
				modTime = parsed
			}
		}
		return FileInfo{Size: info.Size, IsDir: false, ModTime: modTime}, nil
	}
	resolved, err := b.resolvePath(path)
	if err != nil {
		return FileInfo{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime(),
	}, nil
}

func (b *LocalBackend) Exec(ctx context.Context, command string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if b.WorkDir != "" {
		cmd.Dir = b.WorkDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return "", "", -1, err
		}
	}
	return stdout.String(), stderr.String(), exitCode, nil
}
