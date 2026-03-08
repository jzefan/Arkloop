package main

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	shellapi "arkloop/services/sandbox/internal/shell"

	"github.com/klauspost/compress/zstd"
)

type checkpointRoot struct {
	HostPath    string
	ArchivePath string
}

func shellCheckpointRoots() []checkpointRoot {
	return []checkpointRoot{
		{HostPath: shellWorkspaceDir, ArchivePath: "workspace"},
		{HostPath: shellHomeDir, ArchivePath: "home/arkloop"},
		{HostPath: shellTempDir, ArchivePath: "tmp/arkloop"},
	}
}

func (c *ShellController) CheckpointExport() (*shellapi.AgentCheckpointResponse, string, string) {
	c.mu.Lock()
	if c.status == shellapi.StatusClosed || c.cmd == nil {
		c.mu.Unlock()
		return nil, shellapi.CodeSessionNotFound, "shell session not found"
	}
	if c.status == shellapi.StatusRunning {
		c.mu.Unlock()
		return nil, shellapi.CodeSessionBusy, "shell session is busy"
	}
	c.mu.Unlock()

	if _, err := c.runControlCommand("history -a >/dev/null 2>&1 || true", c.cwd, defaultControlTimeout); err != nil {
		return nil, "", err.Error()
	}
	cwd, env, err := c.captureCheckpointState()
	if err != nil {
		return nil, "", err.Error()
	}
	archive, err := exportCheckpointArchive(shellCheckpointRoots())
	if err != nil {
		return nil, "", err.Error()
	}
	return &shellapi.AgentCheckpointResponse{
		Cwd:     cwd,
		Env:     env,
		Archive: base64.StdEncoding.EncodeToString(archive),
	}, "", ""
}

func (c *ShellController) RestoreImport(req shellapi.AgentCheckpointRequest) (*shellapi.AgentCheckpointResponse, string, string) {
	c.mu.Lock()
	busy := c.status != shellapi.StatusClosed || c.cmd != nil || c.ptyFile != nil
	c.mu.Unlock()
	if busy {
		return nil, shellapi.CodeSessionBusy, "shell session is busy"
	}
	payload := strings.TrimSpace(req.Archive)
	if payload == "" {
		return nil, "", "checkpoint archive is required"
	}
	archive, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Sprintf("decode checkpoint archive: %v", err)
	}
	if err := restoreCheckpointArchive(shellCheckpointRoots(), archive); err != nil {
		return nil, "", err.Error()
	}
	return &shellapi.AgentCheckpointResponse{}, "", ""
}

func (c *ShellController) captureCheckpointState() (string, map[string]string, error) {
	if cwd, env, err := c.captureCheckpointStateFromProc(); err == nil {
		return cwd, env, nil
	}
	return c.captureCheckpointStateFromFiles()
}

func (c *ShellController) captureCheckpointStateFromProc() (string, map[string]string, error) {
	if runtime.GOOS != "linux" {
		return "", nil, fmt.Errorf("proc not available")
	}
	c.mu.Lock()
	pid := 0
	fallbackCwd := c.cwd
	if c.cmd != nil && c.cmd.Process != nil {
		pid = c.cmd.Process.Pid
	}
	c.mu.Unlock()
	if pid == 0 {
		return "", nil, fmt.Errorf("shell pid not available")
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return "", nil, fmt.Errorf("read proc cwd: %w", err)
	}
	envRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return "", nil, fmt.Errorf("read proc environ: %w", err)
	}
	env := parseEnvSnapshot(envRaw)
	if cwd == "" {
		cwd = fallbackCwd
	}
	return cwd, env, nil
}

func (c *ShellController) captureCheckpointStateFromFiles() (string, map[string]string, error) {
	tempDir, err := os.MkdirTemp(shellTempDir, "shell-state-")
	if err != nil {
		return "", nil, fmt.Errorf("create checkpoint temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cwdPath := filepath.Join(tempDir, "cwd")
	envPath := filepath.Join(tempDir, "env")
	command := "pwd > " + shellQuote(cwdPath) + " && env -0 > " + shellQuote(envPath)
	if _, err := c.runControlCommand(command, c.cwd, defaultControlTimeout); err != nil {
		return "", nil, err
	}
	cwdRaw, err := os.ReadFile(cwdPath)
	if err != nil {
		return "", nil, fmt.Errorf("read checkpoint cwd: %w", err)
	}
	envRaw, err := os.ReadFile(envPath)
	if err != nil {
		return "", nil, fmt.Errorf("read checkpoint env: %w", err)
	}
	return strings.TrimSpace(string(cwdRaw)), parseEnvSnapshot(envRaw), nil
}

func parseEnvSnapshot(raw []byte) map[string]string {
	result := make(map[string]string)
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		key, value, ok := bytes.Cut(entry, []byte{'='})
		if !ok || len(key) == 0 {
			continue
		}
		result[string(key)] = string(value)
	}
	return result
}

func exportCheckpointArchive(roots []checkpointRoot) ([]byte, error) {
	var buffer bytes.Buffer
	encoder, err := zstd.NewWriter(&buffer)
	if err != nil {
		return nil, fmt.Errorf("create zstd writer: %w", err)
	}
	tarWriter := tar.NewWriter(encoder)
	for _, root := range roots {
		if err := writeCheckpointRoot(tarWriter, root); err != nil {
			tarWriter.Close()
			encoder.Close()
			return nil, err
		}
	}
	if err := tarWriter.Close(); err != nil {
		encoder.Close()
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close zstd writer: %w", err)
	}
	return buffer.Bytes(), nil
}

func writeCheckpointRoot(tw *tar.Writer, root checkpointRoot) error {
	if err := os.MkdirAll(root.HostPath, 0o755); err != nil {
		return fmt.Errorf("ensure checkpoint root %s: %w", root.HostPath, err)
	}
	return filepath.WalkDir(root.HostPath, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		name, err := checkpointArchiveName(root, current)
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			return writeDirHeader(tw, name, info.Mode().Perm(), info.ModTime())
		case mode.IsRegular():
			return writeRegularFileHeader(tw, name, current, info)
		case mode&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(current)
			if err != nil {
				return err
			}
			if !linkTargetWithinRoot(root.HostPath, current, linkTarget) {
				return nil
			}
			return writeSymlinkHeader(tw, name, linkTarget, info.Mode().Perm(), info.ModTime())
		case mode&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0:
			return nil
		default:
			return nil
		}
	})
}

func checkpointArchiveName(root checkpointRoot, current string) (string, error) {
	rel, err := filepath.Rel(root.HostPath, current)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return root.ArchivePath, nil
	}
	return path.Join(root.ArchivePath, filepath.ToSlash(rel)), nil
}

func writeDirHeader(tw *tar.Writer, name string, perm fs.FileMode, modTime time.Time) error {
	if !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeDir,
		Mode:     int64(fileModeOrDefault(int64(perm), 0o755)),
		ModTime:  modTime,
	})
}

func writeRegularFileHeader(tw *tar.Writer, name, fullPath string, info fs.FileInfo) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(tw, file)
	return err
}

func writeSymlinkHeader(tw *tar.Writer, name, linkTarget string, perm fs.FileMode, modTime time.Time) error {
	return tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeSymlink,
		Linkname: linkTarget,
		Mode:     int64(fileModeOrDefault(int64(perm), 0o777)),
		ModTime:  modTime,
	})
}

func linkTargetWithinRoot(rootPath, linkPath, target string) bool {
	if strings.TrimSpace(target) == "" {
		return false
	}
	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(filepath.Dir(linkPath), target)
	}
	resolved = filepath.Clean(resolved)
	return pathWithinRoot(rootPath, resolved)
}

func restoreCheckpointArchive(roots []checkpointRoot, archive []byte) error {
	for _, root := range roots {
		if err := resetCheckpointRoot(root.HostPath); err != nil {
			return err
		}
	}
	decoder, err := zstd.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open checkpoint archive: %w", err)
	}
	defer decoder.Close()
	tr := tar.NewReader(decoder)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			for _, root := range roots {
				if err := chownTreeIfPossible(root.HostPath); err != nil {
					return err
				}
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read checkpoint archive: %w", err)
		}
		root, relPath, err := resolveCheckpointEntry(roots, header.Name)
		if err != nil {
			return err
		}
		targetPath := root.HostPath
		if relPath != "" {
			targetPath = filepath.Join(root.HostPath, filepath.FromSlash(relPath))
		}
		if !pathWithinRoot(root.HostPath, targetPath) && targetPath != root.HostPath {
			return fmt.Errorf("checkpoint entry escapes root: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, fileModeOrDefault(header.Mode, 0o755)); err != nil {
				return fmt.Errorf("restore dir %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			if err := restoreRegularFile(tr, targetPath, header.Mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if !linkTargetWithinRoot(root.HostPath, targetPath, header.Linkname) {
				return fmt.Errorf("checkpoint symlink escapes root: %s", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("restore symlink parent %s: %w", targetPath, err)
			}
			if err := os.RemoveAll(targetPath); err != nil {
				return fmt.Errorf("replace symlink %s: %w", targetPath, err)
			}
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return fmt.Errorf("restore symlink %s: %w", targetPath, err)
			}
		case tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return fmt.Errorf("unsupported checkpoint entry type: %s", header.Name)
		default:
			return fmt.Errorf("unsupported checkpoint entry type: %s", header.Name)
		}
	}
}

func resolveCheckpointEntry(roots []checkpointRoot, name string) (checkpointRoot, string, error) {
	cleaned := path.Clean(strings.TrimSpace(name))
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "../") {
		return checkpointRoot{}, "", fmt.Errorf("invalid checkpoint entry path: %s", name)
	}
	for _, root := range roots {
		if cleaned == root.ArchivePath {
			return root, "", nil
		}
		prefix := root.ArchivePath + "/"
		if strings.HasPrefix(cleaned, prefix) {
			return root, strings.TrimPrefix(cleaned, prefix), nil
		}
	}
	return checkpointRoot{}, "", fmt.Errorf("checkpoint entry outside whitelist: %s", name)
}

func resetCheckpointRoot(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("ensure checkpoint root %s: %w", root, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read checkpoint root %s: %w", root, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return fmt.Errorf("reset checkpoint root %s: %w", root, err)
		}
	}
	return nil
}

func restoreRegularFile(reader io.Reader, targetPath string, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("restore file parent %s: %w", targetPath, err)
	}
	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("replace file %s: %w", targetPath, err)
	}
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileModeOrDefault(mode, 0o644))
	if err != nil {
		return fmt.Errorf("restore file %s: %w", targetPath, err)
	}
	defer file.Close()
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write file %s: %w", targetPath, err)
	}
	return nil
}

func fileModeOrDefault(mode int64, fallback os.FileMode) os.FileMode {
	if mode <= 0 {
		return fallback
	}
	return os.FileMode(mode)
}

func pathWithinRoot(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "..")
}
