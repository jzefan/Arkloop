package bookparser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const DefaultSandboxMaxPages = 0

type RunnerRequest struct {
	MIME     string
	Data     []byte
	MaxPages int
}

type Runner interface {
	Run(ctx context.Context, req RunnerRequest) ([]byte, error)
}

type SandboxParser struct {
	runner   Runner
	maxPages int
}

func NewSandboxParser(runner Runner) *SandboxParser {
	return &SandboxParser{runner: runner, maxPages: DefaultSandboxMaxPages}
}

func (p *SandboxParser) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error) {
	if p.runner == nil {
		return ParsedDoc{}, fmt.Errorf("bookparser: sandbox runner is not configured")
	}
	base := NormalizeMIME(mime)
	if _, ok := sandboxMIMEs[base]; !ok {
		return ParsedDoc{}, fmt.Errorf("%w: %s", ErrUnsupportedMime, mime)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return ParsedDoc{}, err
	}
	out, err := p.runner.Run(ctxOrBackground(ctx), RunnerRequest{MIME: base, Data: raw, MaxPages: p.maxPages})
	if err != nil {
		return ParsedDoc{}, err
	}
	doc, err := decodeParsedDoc(out)
	if err != nil {
		return ParsedDoc{}, err
	}
	if doc.Meta == nil {
		doc.Meta = map[string]any{}
	}
	doc.Meta["source_mime"] = base
	doc.Meta["byte_size"] = len(raw)
	return doc, nil
}

type LocalRunner struct {
	PythonPath string
	ScriptPath string
	Timeout    time.Duration
}

func DefaultLocalRunner() *LocalRunner {
	return &LocalRunner{
		PythonPath: firstNonEmpty(os.Getenv("ARKLOOP_BOOKPARSER_PYTHON"), "python3"),
		ScriptPath: DefaultRunnerPath(),
		Timeout:    2 * time.Minute,
	}
}

func (r *LocalRunner) Run(ctx context.Context, req RunnerRequest) ([]byte, error) {
	script := strings.TrimSpace(r.ScriptPath)
	if script == "" {
		return nil, fmt.Errorf("bookparser: runner script not found; set ARKLOOP_BOOKPARSER_RUNNER")
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctxOrBackground(ctx), timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, firstNonEmpty(r.PythonPath, "python3"),
		script, "--mime", req.MIME, "--max-pages", fmt.Sprintf("%d", req.MaxPages))
	cmd.Stdin = bytes.NewReader(req.Data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("bookparser: sandbox runner timeout: %w", runCtx.Err())
		}
		return nil, fmt.Errorf("bookparser: sandbox runner failed: %s", msg)
	}
	return stdout.Bytes(), nil
}

func DefaultRunnerPath() string {
	if env := strings.TrimSpace(os.Getenv("ARKLOOP_BOOKPARSER_RUNNER")); env != "" {
		return env
	}
	candidates := []string{
		"src/services/shared/bookparser/sandbox/bookparser_runner.py",
		"../shared/bookparser/sandbox/bookparser_runner.py",
		"../../shared/bookparser/sandbox/bookparser_runner.py",
		"bookparser/sandbox/bookparser_runner.py",
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir, i := cwd, 0; i < 8; i++ {
			candidates = append(candidates, filepath.Join(dir, "src/services/shared/bookparser/sandbox/bookparser_runner.py"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

type parsedDocJSON struct {
	Meta   map[string]any `json:"meta"`
	Blocks []blockJSON    `json:"blocks"`
}

type blockJSON struct {
	Type              string         `json:"type"`
	Text              string         `json:"text"`
	HeadingPath       []string       `json:"heading_path"`
	HeadingInferred   bool           `json:"heading_inferred"`
	HeadingConfidence float32        `json:"heading_confidence"`
	Metadata          map[string]any `json:"metadata"`
}

func decodeParsedDoc(raw []byte) (ParsedDoc, error) {
	var payload parsedDocJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedDoc{}, fmt.Errorf("bookparser: decode sandbox json: %w", err)
	}
	blocks := make([]Block, 0, len(payload.Blocks))
	for _, b := range payload.Blocks {
		blockType := BlockType(strings.TrimSpace(b.Type))
		if blockType == "" {
			blockType = BlockParagraph
		}
		blocks = append(blocks, Block{
			Type:              blockType,
			Text:              b.Text,
			HeadingPath:       append([]string(nil), b.HeadingPath...),
			HeadingInferred:   b.HeadingInferred,
			HeadingConfidence: b.HeadingConfidence,
			Metadata:          copyMap(b.Metadata),
		})
	}
	return ParsedDoc{Blocks: blocks, Meta: copyMap(payload.Meta)}, nil
}

func copyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
