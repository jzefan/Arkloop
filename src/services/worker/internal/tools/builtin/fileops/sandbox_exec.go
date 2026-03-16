package fileops

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SandboxExecBackend performs file operations by executing shell commands
// inside a sandbox session through the existing /v1/exec_command HTTP endpoint.
type SandboxExecBackend struct {
	baseURL   string
	authToken string
	sessionID string
	accountID string
	client    *http.Client
}

type sandboxExecRequest struct {
	SessionID string `json:"session_id"`
	AccountID string `json:"account_id,omitempty"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

type sandboxExecResponse struct {
	Output  string `json:"output"`
	Stdout  string `json:"stdout"`
	Status  string `json:"status"`
	Running bool   `json:"running"`
}

func (b *SandboxExecBackend) httpClient() *http.Client {
	if b.client != nil {
		return b.client
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (b *SandboxExecBackend) exec(ctx context.Context, command string, timeoutMs int) (string, error) {
	if timeoutMs == 0 {
		timeoutMs = 30_000
	}
	payload, err := json.Marshal(sandboxExecRequest{
		SessionID: b.sessionID,
		AccountID: b.accountID,
		Command:   command,
		TimeoutMs: timeoutMs,
	})
	if err != nil {
		return "", fmt.Errorf("marshal sandbox request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/v1/exec_command", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build sandbox request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}
	if b.accountID != "" {
		req.Header.Set("X-Account-ID", b.accountID)
	}

	resp, err := b.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("sandbox request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read sandbox response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sandbox returned %d: %s", resp.StatusCode, string(body))
	}

	var result sandboxExecResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode sandbox response: %w", err)
	}
	output := result.Output
	if output == "" {
		output = result.Stdout
	}
	return output, nil
}

func (b *SandboxExecBackend) ReadFile(ctx context.Context, path string) ([]byte, error) {
	output, err := b.exec(ctx, fmt.Sprintf("cat %s", shellQuote(path)), 30_000)
	if err != nil {
		return nil, err
	}
	return []byte(output), nil
}

func (b *SandboxExecBackend) WriteFile(ctx context.Context, path string, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)
	dir := path[:strings.LastIndex(path, "/")]
	cmd := fmt.Sprintf("mkdir -p %s && echo %s | base64 -d > %s",
		shellQuote(dir), shellQuote(encoded), shellQuote(path))
	_, err := b.exec(ctx, cmd, 30_000)
	return err
}

func (b *SandboxExecBackend) Stat(ctx context.Context, path string) (FileInfo, error) {
	// GNU stat: %s=size, %F=file type, %Y=mtime epoch
	output, err := b.exec(ctx, fmt.Sprintf("stat -c '%%s %%F %%Y' %s 2>/dev/null || stat -f '%%z %%HT %%m' %s", shellQuote(path), shellQuote(path)), 10_000)
	if err != nil {
		return FileInfo{}, err
	}
	return parseStat(strings.TrimSpace(output))
}

func (b *SandboxExecBackend) Exec(ctx context.Context, command string) (string, string, int, error) {
	output, err := b.exec(ctx, command, 60_000)
	if err != nil {
		return "", "", -1, err
	}
	return output, "", 0, nil
}

func parseStat(line string) (FileInfo, error) {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return FileInfo{}, fmt.Errorf("unexpected stat output: %q", line)
	}
	size, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return FileInfo{}, fmt.Errorf("parse size: %w", err)
	}
	isDir := strings.Contains(strings.ToLower(parts[1]), "directory")
	epoch, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return FileInfo{}, fmt.Errorf("parse mtime: %w", err)
	}
	return FileInfo{
		Size:    size,
		IsDir:   isDir,
		ModTime: time.Unix(epoch, 0),
	}, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
