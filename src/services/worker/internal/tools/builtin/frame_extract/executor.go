// Package frameextract extracts a single PNG frame from an mp4 artifact using
// the local ffmpeg binary. Used by yuhua-stone-director to chain shot
// continuity: shot N's last frame becomes shot N+1's first_frame.
package frameextract

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

const (
	defaultArtifactName = "extracted_frame"
	ffmpegTimeout       = 60 * time.Second
)

type ToolExecutor struct {
	store objectstore.Store
}

func NewToolExecutor(store objectstore.Store) *ToolExecutor {
	return &ToolExecutor{store: store}
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if e == nil || e.store == nil {
		return errResult("tool.not_configured", "frame_extract storage is not configured", started)
	}
	if execCtx.AccountID == nil {
		return errResult("tool.execution_failed", "account context is required", started)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errResult("tool.not_configured", "ffmpeg binary not found in worker image", started)
	}

	input := strings.TrimSpace(stringArg(args, "input"))
	if input == "" {
		return errResult("tool.args_invalid", "parameter input is required", started)
	}
	key := strings.TrimSpace(strings.TrimPrefix(input, "artifact:"))
	if !strings.HasPrefix(key, execCtx.AccountID.String()+"/") {
		return errResult("tool.args_invalid", "input is outside the current account", started)
	}

	// Resolve position: at_seconds > at (first/last) > default last
	position := "last"
	var atSeconds *float64
	if raw, ok := args["at"].(string); ok && strings.TrimSpace(raw) != "" {
		position = strings.ToLower(strings.TrimSpace(raw))
	}
	if raw, ok := args["at_seconds"]; ok {
		switch v := raw.(type) {
		case float64:
			atSeconds = &v
		case int:
			f := float64(v)
			atSeconds = &f
		}
	}

	tmpDir, err := os.MkdirTemp("", "frame_extract-")
	if err != nil {
		return errResult("tool.execution_failed", "create tmp: "+err.Error(), started)
	}
	defer os.RemoveAll(tmpDir)

	data, _, err := e.store.GetWithContentType(ctx, key)
	if err != nil {
		return errResult("tool.execution_failed", "download input: "+err.Error(), started)
	}
	inPath := filepath.Join(tmpDir, "in.mp4")
	if err := os.WriteFile(inPath, data, 0o600); err != nil {
		return errResult("tool.execution_failed", "write tmp mp4: "+err.Error(), started)
	}
	outPath := filepath.Join(tmpDir, "out.png")

	// Build ffmpeg args. Last-frame extraction uses -sseof -0.1 to seek to a
	// fractional second before EOF and grab the last decoded frame; this is more
	// reliable than -update 1 alone because some seedance mp4s have B-frames
	// that confuse -update at EOF.
	ffCtx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()
	var ffArgs []string
	switch {
	case atSeconds != nil:
		ffArgs = []string{
			"-y", "-hide_banner", "-loglevel", "error",
			"-ss", fmt.Sprintf("%.3f", *atSeconds),
			"-i", inPath,
			"-frames:v", "1",
			outPath,
		}
	case position == "first":
		ffArgs = []string{
			"-y", "-hide_banner", "-loglevel", "error",
			"-i", inPath,
			"-frames:v", "1",
			outPath,
		}
	default: // "last"
		// -sseof seeks from end; -0.1 = 100ms before end. -update overwrites.
		ffArgs = []string{
			"-y", "-hide_banner", "-loglevel", "error",
			"-sseof", "-0.1",
			"-i", inPath,
			"-update", "1",
			"-frames:v", "1",
			outPath,
		}
	}

	cmd := exec.CommandContext(ffCtx, "ffmpeg", ffArgs...)
	stderr, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return errResult("tool.execution_failed",
			fmt.Sprintf("ffmpeg frame extract failed: %s stderr=%s", runErr.Error(), truncate(string(stderr), 400)),
			started)
	}

	pngBytes, err := os.ReadFile(outPath)
	if err != nil {
		return errResult("tool.execution_failed", "read extracted frame: "+err.Error(), started)
	}
	if len(pngBytes) == 0 {
		return errResult("tool.execution_failed", "extracted frame is empty", started)
	}

	artifactBase := sanitizeArtifactName(strings.TrimSpace(stringArg(args, "artifact_name")))
	if artifactBase == "" {
		artifactBase = defaultArtifactName
	}
	filename := artifactBase + ".png"
	outKey := fmt.Sprintf("%s/%s/%s", execCtx.AccountID.String(), execCtx.RunID.String(), filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		v := execCtx.ThreadID.String()
		threadID = &v
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), execCtx.AccountID.String(), threadID)
	if err := e.store.PutObject(ctx, outKey, pngBytes, objectstore.PutOptions{
		ContentType: "image/png",
		Metadata:    metadata,
	}); err != nil {
		return errResult("tool.upload_failed", "save extracted frame: "+err.Error(), started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"mime_type": "image/png",
			"bytes":     len(pngBytes),
			"artifacts": []map[string]any{
				{
					"key":       outKey,
					"filename":  filename,
					"size":      len(pngBytes),
					"mime_type": "image/png",
					"title":     artifactBase,
					"display":   "inline",
				},
			},
		},
		DurationMs: durationMs(started),
	}
}

func (e *ToolExecutor) IsAvailableForAccount(_ context.Context, accountID uuid.UUID) bool {
	if accountID == uuid.Nil {
		return false
	}
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func sanitizeArtifactName(raw string) string {
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteByte('_')
		}
	}
	cleaned := strings.Trim(b.String(), "-_")
	if len(cleaned) > 80 {
		cleaned = cleaned[:80]
	}
	return cleaned
}

func errResult(class, msg string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error:      &tools.ExecutionError{ErrorClass: class, Message: msg},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	d := time.Since(started)
	if d < 0 {
		return 0
	}
	return int(d / time.Millisecond)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
