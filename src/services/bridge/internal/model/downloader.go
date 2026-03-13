package model

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"arkloop/services/bridge/internal/docker"
)

// Variant describes a downloadable model variant.
type Variant struct {
	ID    string // "22m" or "86m"
	Name  string
	Image string // OCI image containing model files
	Size  string // human-readable size hint
}

// Variants is the registry of known prompt-guard model variants.
// Image names follow: ghcr.io/arkloop/prompt-guard-{variant}-onnx:latest
// Each image stores model.onnx + tokenizer.json under /models/.
var Variants = map[string]Variant{
	"22m": {
		ID:    "22m",
		Name:  "Prompt Guard 2 (22M)",
		Image: "ghcr.io/arkloop/prompt-guard-22m-onnx:latest",
		Size:  "~22 MB",
	},
	"86m": {
		ID:    "86m",
		Name:  "Prompt Guard (86M)",
		Image: "ghcr.io/arkloop/prompt-guard-86m-onnx:latest",
		Size:  "~88 MB",
	},
}

// Logger is the interface for structured logging.
type Logger interface {
	Info(msg string, extra map[string]any)
	Error(msg string, extra map[string]any)
}

// Downloader handles model file distribution via OCI images.
type Downloader struct {
	modelDir string
	logger   Logger
	mu       sync.Mutex
	busy     bool
}

// NewDownloader creates a model downloader targeting the given directory.
// If modelDir is empty it defaults to /var/lib/arkloop/models/prompt-guard.
func NewDownloader(modelDir string, logger Logger) *Downloader {
	if modelDir == "" {
		dataDir := os.Getenv("ARKLOOP_DATA_DIR")
		if dataDir == "" {
			dataDir = "/var/lib/arkloop"
		}
		modelDir = dataDir + "/models/prompt-guard"
	}
	return &Downloader{modelDir: modelDir, logger: logger}
}

// Install pulls the model variant image, extracts model files, and writes
// them to the configured model directory. Returns an Operation that can be
// tracked via the operation stream endpoint.
func (d *Downloader) Install(ctx context.Context, variantID string) (*docker.Operation, error) {
	v, ok := Variants[variantID]
	if !ok {
		return nil, fmt.Errorf("unknown model variant %q (valid: 22m, 86m)", variantID)
	}

	// Allow overriding the image via env var for dev/testing.
	if override := os.Getenv("ARKLOOP_PROMPT_GUARD_IMAGE_" + strings.ToUpper(variantID)); override != "" {
		v.Image = override
	}

	d.mu.Lock()
	if d.busy {
		d.mu.Unlock()
		return nil, fmt.Errorf("prompt-guard model install already in progress")
	}
	d.busy = true
	d.mu.Unlock()

	op := docker.NewOperation("prompt-guard", "install")
	op.Status = docker.OperationRunning

	cancelCtx, cancel := context.WithCancel(ctx)
	op.SetCancelFunc(cancel)

	go func() {
		defer func() {
			d.mu.Lock()
			d.busy = false
			d.mu.Unlock()
		}()
		err := d.download(cancelCtx, op, v)
		op.Complete(err)
	}()

	return op, nil
}

func (d *Downloader) download(ctx context.Context, op *docker.Operation, v Variant) error {
	op.AppendLog(fmt.Sprintf("variant: %s (%s)", v.Name, v.Size))

	// 1. Create target directory
	if err := os.MkdirAll(d.modelDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", d.modelDir, err)
	}
	op.AppendLog("model directory: " + d.modelDir)

	// 2. Pull image
	op.AppendLog(fmt.Sprintf("pulling %s ...", v.Image))
	if err := d.run(ctx, op, "docker", "pull", v.Image); err != nil {
		return fmt.Errorf("docker pull: %w", err)
	}

	// 3. Create temporary container
	containerName := "arkloop-model-extract-" + v.ID
	// Remove any leftover container from a previous failed run.
	_ = d.run(ctx, op, "docker", "rm", "-f", containerName)

	op.AppendLog("extracting model files...")
	if err := d.run(ctx, op, "docker", "create", "--name", containerName, v.Image); err != nil {
		return fmt.Errorf("docker create: %w", err)
	}

	// 4. Copy model files out
	src := containerName + ":/models/."
	if err := d.run(ctx, op, "docker", "cp", src, d.modelDir); err != nil {
		_ = d.run(ctx, op, "docker", "rm", "-f", containerName)
		return fmt.Errorf("docker cp: %w", err)
	}

	// 5. Cleanup
	_ = d.run(ctx, op, "docker", "rm", "-f", containerName)

	// 6. Verify essential files exist
	for _, f := range []string{"model.onnx", "tokenizer.json"} {
		path := d.modelDir + "/" + f
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("expected file missing after extract: %s", path)
		}
	}

	op.AppendLog("model installed to " + d.modelDir)
	d.logger.Info("prompt-guard model installed", map[string]any{
		"variant":   v.ID,
		"model_dir": d.modelDir,
	})
	return nil
}

// run executes a command, streams stdout/stderr to the operation log, and
// returns the exit error (if any).
func (d *Downloader) run(ctx context.Context, op *docker.Operation, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return err
	}
	if cmd.Process != nil {
		op.SetPID(cmd.Process.Pid)
	}

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		op.AppendLog(scanner.Text())
	}
	return cmd.Wait()
}
