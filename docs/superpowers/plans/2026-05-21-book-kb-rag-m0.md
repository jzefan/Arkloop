# Book→KB→RAG M0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a vertical slice that ingests plain-text input through chunker → Doubao embeddings → pgvector and retrieves top-k chunks via debug HTTP endpoints, proving the pipeline before M1 invests in PDF parsing / persona / UI.

**Architecture:** All new code lives in `src/services/shared/{bookchunker,embedding}/` (reusable in M1 worker) and `src/services/api/internal/data/kb_chunks_repo.go` + `internal/http/kbdebugapi/`. The Doubao embedder uses the existing `defaultDoubaoBaseURL` and credentials wiring. Pgvector ships via a new goose migration that depends on the postgres image being switched to `pgvector/pgvector:pg16`. Endpoints sit behind a constant-time Bearer-token middleware modelled on `oauthapi/helpers.go:validateServiceToken`.

**Tech Stack:** Go 1.26, `pgx/v5`, pgvector 0.7+, Doubao Ark API (OpenAI-compatible `/embeddings`), goose migrations, `testutil.SetupPostgresDatabase` for integration tests, tiktoken-go for token counting.

**Reference spec:** `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md`

**Out of scope (do NOT implement in this plan):** PDF parsing, persona, UI, QuestionStore, knowledge_bases / kb_documents tables, Workspace auth, worker tools, job queue, multi-user concurrency, ACL beyond debug token, RAG question generation.

---

## Task 0: Spike S0 — Probe Doubao embedding dimensions

**Why:** Pgvector's `vector(N)` column type bakes the dimension into DDL. Changing N later requires drop+rebuild. Two contradictory candidate values exist (`openviking_resolver.go:365` says 1024, design doc previously assumed 2560). Must hit the real API to pick N.

**Files:**
- Create: `src/services/api/cmd/embedprobe/main.go`

**Steps:**

- [ ] **Step 1: Write the probe program**

Create `src/services/api/cmd/embedprobe/main.go`:

```go
// Command embedprobe issues a single embedding request against the configured
// Doubao endpoint and prints the returned vector's dimension. Used during M0
// to fix pgvector(N) before applying any migration.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func main() {
	model := flag.String("model", "doubao-embedding-text-240715", "embedding model id")
	baseURL := flag.String("base-url", "https://ark.cn-beijing.volces.com/api/v3", "ark base url")
	input := flag.String("input", "光的干涉是指两列或多列频率相同的光波相遇时发生的现象。", "text to embed")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
	flag.Parse()

	apiKey := os.Getenv("ARK_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ARK_API_KEY env required")
		os.Exit(2)
	}

	body, _ := json.Marshal(embedReq{Model: *model, Input: []string{*input}})
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, *baseURL+"/embeddings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "non-200 (%d): %s\n", resp.StatusCode, string(raw))
		os.Exit(1)
	}

	var parsed embedResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		fmt.Fprintln(os.Stderr, "parse failed:", err)
		os.Exit(1)
	}
	if len(parsed.Data) == 0 {
		fmt.Fprintln(os.Stderr, "empty data")
		os.Exit(1)
	}
	fmt.Printf("model=%s dim=%d latency_ms=%d prompt_tokens=%d\n",
		parsed.Model, len(parsed.Data[0].Embedding), time.Since(start).Milliseconds(), parsed.Usage.PromptTokens)
}
```

- [ ] **Step 2: Run the probe (operator action, not CI)**

```bash
cd src/services/api
export ARK_API_KEY=<your doubao key>
go run ./cmd/embedprobe -model doubao-embedding-text-240715
```

Expected output line shape: `model=doubao-embedding-text-240715 dim=<N> latency_ms=<X> prompt_tokens=<Y>`

Record the `dim=<N>` value. It will be substituted everywhere this plan says `<DOUBAO_DIM>`.

- [ ] **Step 3: Probe batch limits (sanity probe)**

```bash
# Try batches of 1, 8, 32, 64 — note any rejection
for n in 1 8 32 64; do
  echo "=== batch $n ==="
  # Quick inline script — duplicate input n times via shell loop
done
```

Record the max batch size that returns 200. Default in code (Task 2) will be `min(observed_max, 32)`.

- [ ] **Step 4: Update the design doc with probed values**

Edit `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md`, find the "已锁决策" table row for **Embedding 模型**, replace `维度待 S0 probe 确认（候选记忆值 2560，未验证）` with `维度 <N>（S0 probed YYYY-MM-DD），单批上限 <M>，latency baseline <X> ms`.

- [ ] **Step 5: Commit**

```bash
git add src/services/api/cmd/embedprobe/main.go \
        docs/superpowers/specs/2026-05-21-book-kb-rag-design.md
git commit -m "feat(api): add embedprobe cmd and record S0 results

Probe Doubao doubao-embedding-text-240715 dimensions/latency/batch
limits to fix pgvector(N) before migration."
```

---

## Task 1: bookchunker package — token counter + chunk algorithm

**Files:**
- Create: `src/services/shared/bookchunker/chunker.go`
- Create: `src/services/shared/bookchunker/chunker_test.go`
- Create: `src/services/shared/bookchunker/testdata/cn_textbook_excerpt.txt`
- Modify: `src/services/shared/go.mod` (add tiktoken-go dep)

**Steps:**

- [ ] **Step 1: Add tiktoken-go dependency**

```bash
cd src/services/shared
go get github.com/pkoukk/tiktoken-go@latest
go mod tidy
```

Run: `go list -m github.com/pkoukk/tiktoken-go`
Expected: prints a version line.

- [ ] **Step 2: Write the failing test**

Create `src/services/shared/bookchunker/chunker_test.go`:

```go
package bookchunker

import (
	"strings"
	"testing"
)

// 中文教科书片段，约 2000 字符，3 个段落（两个空行分隔）。
const sampleParagraph = "光的干涉是指两列或多列频率相同的光波相遇时发生的现象，其结果是某些位置振幅相互加强，另一些位置振幅相互削弱，从而形成稳定的明暗条纹。1801 年托马斯·杨通过双缝实验首次证明了光具有波动性，实验中双缝间距、缝至屏的距离以及光的波长共同决定了条纹间距，可以由公式 Δy = λL/d 计算。"

func TestChunkSinglePassParagraph(t *testing.T) {
	text := strings.Repeat(sampleParagraph, 6) // ~5–6k Chinese chars => > 512 tokens
	chunks, err := Chunk(text, DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks for long input, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.TokenCount > DefaultOptions().MaxTokens {
			t.Errorf("chunk %d exceeds MaxTokens: got %d > %d", i, c.TokenCount, DefaultOptions().MaxTokens)
		}
		if c.TokenCount < DefaultOptions().MinTokens && i != len(chunks)-1 {
			t.Errorf("non-tail chunk %d below MinTokens: got %d < %d", i, c.TokenCount, DefaultOptions().MinTokens)
		}
		if c.Text == "" {
			t.Errorf("chunk %d empty", i)
		}
		if c.Ordinal != i {
			t.Errorf("chunk %d ordinal mismatch: got %d", i, c.Ordinal)
		}
	}
}

func TestChunkOverlap(t *testing.T) {
	text := strings.Repeat(sampleParagraph, 6)
	chunks, _ := Chunk(text, DefaultOptions())
	if len(chunks) < 2 {
		t.Skip("need >=2 chunks for overlap test")
	}
	// Last 20 runes of chunk N should appear in chunk N+1's first 80 runes (loose overlap check).
	prevTail := []rune(chunks[0].Text)
	if len(prevTail) > 30 {
		prevTail = prevTail[len(prevTail)-30:]
	}
	head := []rune(chunks[1].Text)
	if len(head) > 120 {
		head = head[:120]
	}
	if !strings.Contains(string(head), string(prevTail[len(prevTail)/2:])) {
		t.Errorf("expected overlap between chunk 0 tail and chunk 1 head")
	}
}

func TestChunkShortInputReturnsSingleChunk(t *testing.T) {
	chunks, err := Chunk("短句。", DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short input, got %d", len(chunks))
	}
	if chunks[0].Text != "短句。" {
		t.Errorf("unexpected text: %q", chunks[0].Text)
	}
}

func TestChunkEmptyInputReturnsEmpty(t *testing.T) {
	chunks, err := Chunk("", DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkDeterministic(t *testing.T) {
	text := strings.Repeat(sampleParagraph, 6)
	a, _ := Chunk(text, DefaultOptions())
	b, _ := Chunk(text, DefaultOptions())
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Text != b[i].Text || a[i].TokenCount != b[i].TokenCount {
			t.Fatalf("chunk %d differs across runs", i)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail with "Chunk not defined"**

```bash
cd src/services/shared
go test ./bookchunker/...
```

Expected: compile error (`undefined: Chunk`, `undefined: DefaultOptions`).

- [ ] **Step 4: Implement chunker**

Create `src/services/shared/bookchunker/chunker.go`:

```go
// Package bookchunker splits long text into overlapping chunks suitable for
// embedding-based retrieval. Pure function; no I/O. M0 inputs are paragraph-
// separated plain text; M1 will extend the signature with a structured
// ParsedDoc input but keep the chunk output shape stable.
package bookchunker

import (
	"fmt"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// Chunk is one output unit. Ordinal is 0-based source order.
type Chunk struct {
	Ordinal    int
	Text       string
	TokenCount int
}

// ChunkOptions tunes split behavior. Use DefaultOptions for M0.
type ChunkOptions struct {
	MinTokens     int
	MaxTokens     int
	OverlapTokens int
	Encoding      string // tiktoken encoding name; cl100k_base is the M0 default
}

// DefaultOptions returns the M0 default parameters.
func DefaultOptions() ChunkOptions {
	return ChunkOptions{
		MinTokens:     256,
		MaxTokens:     512,
		OverlapTokens: 40,
		Encoding:      "cl100k_base",
	}
}

// Chunk splits text into chunks per opts. Empty input returns nil.
func Chunk(text string, opts ChunkOptions) ([]Chunk, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	enc, err := tiktoken.GetEncoding(opts.Encoding)
	if err != nil {
		return nil, fmt.Errorf("load tiktoken encoding %q: %w", opts.Encoding, err)
	}

	paragraphs := splitParagraphs(text)
	tokens := make([][]int, len(paragraphs))
	totalTokens := 0
	for i, p := range paragraphs {
		tokens[i] = enc.Encode(p, nil, nil)
		totalTokens += len(tokens[i])
	}
	if totalTokens <= opts.MaxTokens {
		return []Chunk{{
			Ordinal:    0,
			Text:       strings.Join(paragraphs, "\n\n"),
			TokenCount: totalTokens,
		}}, nil
	}

	// Flatten paragraph tokens for sliding-window with overlap.
	flat := make([]int, 0, totalTokens)
	for _, t := range tokens {
		flat = append(flat, t...)
	}

	var chunks []Chunk
	pos := 0
	for pos < len(flat) {
		end := pos + opts.MaxTokens
		if end > len(flat) {
			end = len(flat)
		}
		window := flat[pos:end]
		chunks = append(chunks, Chunk{
			Ordinal:    len(chunks),
			Text:       enc.Decode(window),
			TokenCount: len(window),
		})
		if end == len(flat) {
			break
		}
		// Advance by MaxTokens - OverlapTokens, but never less than MinTokens to make progress.
		step := opts.MaxTokens - opts.OverlapTokens
		if step < opts.MinTokens {
			step = opts.MinTokens
		}
		pos += step
	}
	return chunks, nil
}

// splitParagraphs splits on blank lines, trimming each paragraph.
func splitParagraphs(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 5: Run tests, expect pass**

```bash
cd src/services/shared
go test ./bookchunker/... -v
```

Expected: all 5 tests PASS.

- [ ] **Step 6: Add a golden fixture file (for human eyeballing, not test asserts)**

Create `src/services/shared/bookchunker/testdata/cn_textbook_excerpt.txt` with a real 3-paragraph 大学物理 excerpt (~1500 chars; jzefan supplies). This is for the M0 end-to-end integration test in Task 8, kept under testdata so `go test` doesn't choke on it as a package.

- [ ] **Step 7: Commit**

```bash
git add src/services/shared/bookchunker src/services/shared/go.mod src/services/shared/go.sum
git commit -m "feat(shared): add bookchunker pkg for sliding-window token chunking

Pure function with TDD coverage: short input passthrough, long input
multi-chunk with overlap, deterministic, empty input safe. Uses
tiktoken cl100k_base for token counts."
```

---

## Task 2: embedding package — Doubao Ark backend

**Files:**
- Create: `src/services/shared/embedding/embedder.go`
- Create: `src/services/shared/embedding/doubao.go`
- Create: `src/services/shared/embedding/doubao_test.go`

**Steps:**

- [ ] **Step 1: Write the failing test (against httptest server)**

Create `src/services/shared/embedding/doubao_test.go`:

```go
package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newFakeArk(t *testing.T, dim int, failuresBeforeSuccess int32) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	var failsLeft = failuresBeforeSuccess
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/embeddings" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		if atomic.LoadInt32(&failsLeft) > 0 {
			atomic.AddInt32(&failsLeft, -1)
			http.Error(w, "transient", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		resp := map[string]any{
			"model": "doubao-embedding-text-240715",
			"data":  []any{},
		}
		data := make([]any, len(body.Input))
		for i := range body.Input {
			vec := make([]float32, dim)
			vec[0] = float32(i + 1)
			data[i] = map[string]any{"index": i, "embedding": vec}
		}
		resp["data"] = data
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestDoubaoEmbedSingleBatch(t *testing.T) {
	srv, _ := newFakeArk(t, 1024, 0)
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "doubao-embedding-text-240715",
		BatchSize: 32, MaxRetries: 0, Dim: 1024,
	})
	vecs, err := emb.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("got %d vecs, want 3", len(vecs))
	}
	if len(vecs[0]) != 1024 {
		t.Fatalf("dim mismatch: %d", len(vecs[0]))
	}
}

func TestDoubaoEmbedBatchesWhenOverBatchSize(t *testing.T) {
	srv, calls := newFakeArk(t, 16, 0)
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 2, MaxRetries: 0, Dim: 16,
	})
	in := []string{"a", "b", "c", "d", "e"} // 5 inputs, BatchSize 2 => 3 calls
	vecs, err := emb.Embed(context.Background(), in)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 5 {
		t.Fatalf("got %d vecs, want 5", len(vecs))
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("expected 3 HTTP calls, got %d", got)
	}
}

func TestDoubaoEmbedRetriesOnTransient(t *testing.T) {
	srv, calls := newFakeArk(t, 8, 2) // 2 transient failures, then success
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 32, MaxRetries: 3, BaseBackoff: 1 * time.Millisecond, Dim: 8,
	})
	_, err := emb.Embed(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestDoubaoEmbedReturnsErrAfterMaxRetries(t *testing.T) {
	srv, _ := newFakeArk(t, 8, 99) // always fail
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 32, MaxRetries: 2, BaseBackoff: 1 * time.Millisecond, Dim: 8,
	})
	_, err := emb.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected error after retry budget")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("expected ErrUpstream, got %v", err)
	}
}

func TestDoubaoEmbedEmptyInputReturnsEmpty(t *testing.T) {
	emb := NewDoubao(DoubaoConfig{BaseURL: "http://unused", APIKey: "k", Model: "m", BatchSize: 32, Dim: 8})
	vecs, err := emb.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Fatalf("empty input: got vecs=%v err=%v", vecs, err)
	}
}

func TestDoubaoRejectsDimMismatch(t *testing.T) {
	srv, _ := newFakeArk(t, 1024, 0) // server returns 1024
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 32, MaxRetries: 0, Dim: 2560, // expect 2560
	})
	_, err := emb.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected dim-mismatch error")
	}
	if !errors.Is(err, ErrDimMismatch) {
		t.Errorf("expected ErrDimMismatch, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests, expect compile failure**

```bash
cd src/services/shared
go test ./embedding/...
```

Expected: undefined symbols.

- [ ] **Step 3: Implement Embedder interface**

Create `src/services/shared/embedding/embedder.go`:

```go
// Package embedding provides text→vector embeddings. M0 implements the
// Doubao Ark backend; the interface is named so M1 can add OpenAI /
// OpenViking implementations without breaking callers.
package embedding

import (
	"context"
	"errors"
)

// Embedder turns text into fixed-dimension float32 vectors.
type Embedder interface {
	// Embed returns one vector per input in input order. nil input → nil output.
	// Returned vectors are guaranteed to have length Dim().
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dim is the dimension of every vector this Embedder returns.
	Dim() int
}

// ErrUpstream indicates the upstream provider failed after the retry budget.
var ErrUpstream = errors.New("embedding: upstream failed after retries")

// ErrDimMismatch indicates the server returned a vector of a different size
// than the configured Dim. This protects pgvector(N) against silent drift.
var ErrDimMismatch = errors.New("embedding: provider returned unexpected dimension")
```

- [ ] **Step 4: Implement Doubao backend**

Create `src/services/shared/embedding/doubao.go`:

```go
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DoubaoConfig configures the Doubao Ark embedding backend. Dim is the
// authoritative expected dimension; any response of a different size is
// rejected with ErrDimMismatch to guard pgvector(N).
type DoubaoConfig struct {
	BaseURL     string        // e.g. https://ark.cn-beijing.volces.com/api/v3
	APIKey      string        // Authorization: Bearer <APIKey>
	Model       string        // e.g. doubao-embedding-text-240715
	BatchSize   int           // ≤ 32 (S0 probed value)
	MaxRetries  int           // retries on 5xx and transport errors
	BaseBackoff time.Duration // first retry waits BaseBackoff; later doubles
	Timeout     time.Duration // per-request timeout (default 30s)
	Dim         int           // expected vector length, fixed by S0
	HTTPClient  *http.Client
}

// Doubao implements Embedder against an OpenAI-compatible /embeddings endpoint.
type Doubao struct {
	cfg DoubaoConfig
	hc  *http.Client
}

// NewDoubao builds a Doubao embedder. Caller must have run Spike S0 to know Dim.
func NewDoubao(cfg DoubaoConfig) *Doubao {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 32
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 250 * time.Millisecond
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	return &Doubao{cfg: cfg, hc: hc}
}

func (d *Doubao) Dim() int { return d.cfg.Dim }

type arkReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type arkResp struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed implements Embedder.
func (d *Doubao) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += d.cfg.BatchSize {
		end := start + d.cfg.BatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[start:end]
		vecs, err := d.embedOnce(ctx, batch)
		if err != nil {
			return nil, err
		}
		copy(out[start:end], vecs)
	}
	return out, nil
}

func (d *Doubao) embedOnce(ctx context.Context, batch []string) ([][]float32, error) {
	body, _ := json.Marshal(arkReq{Model: d.cfg.Model, Input: batch})
	var lastErr error
	for attempt := 0; attempt <= d.cfg.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+d.cfg.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := d.hc.Do(req)
		if err != nil {
			lastErr = err
		} else {
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				var parsed arkResp
				if err := json.Unmarshal(raw, &parsed); err != nil {
					return nil, fmt.Errorf("decode ark response: %w", err)
				}
				if len(parsed.Data) != len(batch) {
					return nil, fmt.Errorf("ark returned %d vectors, want %d", len(parsed.Data), len(batch))
				}
				vecs := make([][]float32, len(batch))
				for i, item := range parsed.Data {
					if len(item.Embedding) != d.cfg.Dim {
						return nil, fmt.Errorf("%w: got %d want %d", ErrDimMismatch, len(item.Embedding), d.cfg.Dim)
					}
					vecs[item.Index] = item.Embedding
					_ = i
				}
				return vecs, nil
			}
			lastErr = fmt.Errorf("ark http %d: %s", resp.StatusCode, string(raw))
			// Retry only on 5xx; client errors are terminal.
			if resp.StatusCode < 500 {
				return nil, lastErr
			}
		}
		// Backoff before next attempt.
		if attempt < d.cfg.MaxRetries {
			wait := d.cfg.BaseBackoff << attempt
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, fmt.Errorf("%w: %v", ErrUpstream, lastErr)
}
```

- [ ] **Step 5: Run tests, expect pass**

```bash
cd src/services/shared
go test ./embedding/... -v
```

Expected: all 6 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add src/services/shared/embedding
git commit -m "feat(shared): add embedding pkg with Doubao Ark backend

Embedder interface (Embed + Dim) with Doubao implementation:
batching, exponential backoff on 5xx, dim-mismatch guard for
pgvector safety. Unit tests use httptest fake Ark server."
```

---

## Task 3: pgvector migration (gated by S0 result)

**Files:**
- Create: `src/services/api/internal/migrate/migrations/00192_kb_chunks.sql`

**Steps:**

- [ ] **Step 1: Write the migration**

Create `src/services/api/internal/migrate/migrations/00192_kb_chunks.sql`:

> Replace `<DOUBAO_DIM>` with the integer from Task 0 Step 2 output (e.g. 1024 or 2560). Do not commit this file until Task 0 is finished and the value is in hand.

```sql
-- +goose Up

-- M0 of book-kb-rag: minimal schema to validate the chunker → embedder →
-- pgvector → retrieval pipeline end-to-end. M1 will introduce
-- knowledge_bases, kb_documents, etc. and rebuild this table with
-- foreign keys; that migration will copy/transform existing rows.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE kb_chunks (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_name       TEXT         NOT NULL,
    document_ref  TEXT         NOT NULL,
    ordinal       INTEGER      NOT NULL,
    text          TEXT         NOT NULL,
    token_count   INTEGER      NOT NULL,
    embedding     vector(<DOUBAO_DIM>) NOT NULL,
    metadata_json JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),

    UNIQUE (kb_name, document_ref, ordinal)
);

CREATE INDEX kb_chunks_kb_name_idx ON kb_chunks (kb_name);

-- hnsw with cosine ops. ef_construction and m are pgvector defaults;
-- M0 data volume is small so we don't tune these.
CREATE INDEX kb_chunks_embedding_hnsw_idx
    ON kb_chunks
    USING hnsw (embedding vector_cosine_ops);

-- +goose Down

DROP INDEX IF EXISTS kb_chunks_embedding_hnsw_idx;
DROP INDEX IF EXISTS kb_chunks_kb_name_idx;
DROP TABLE IF EXISTS kb_chunks;
-- Note: we do NOT drop the vector extension on rollback; other migrations
-- in the future may depend on it.
```

- [ ] **Step 2: Verify migration applies cleanly against a pgvector image**

```bash
# Spin up a one-off pgvector pg16 container locally
docker run --rm -d --name pgv-test -e POSTGRES_PASSWORD=test -p 5599:5432 pgvector/pgvector:pg16
# Wait ~3s for postgres readiness
sleep 3
# Apply all migrations including the new one
cd src/services/api
ARKLOOP_DATABASE_URL=postgres://postgres:test@localhost:5599/postgres?sslmode=disable \
  go run ./cmd/migrate up
# Confirm
docker exec pgv-test psql -U postgres -c "\d kb_chunks"
docker stop pgv-test
```

Expected: `\d kb_chunks` prints the table with `embedding | vector(<DOUBAO_DIM>)` and the two indexes.

- [ ] **Step 3: Commit**

```bash
git add src/services/api/internal/migrate/migrations/00192_kb_chunks.sql
git commit -m "feat(api): add pgvector kb_chunks table for M0 KB pipeline

vector(<DOUBAO_DIM>) sized from Spike S0 probe of Doubao
doubao-embedding-text-240715. hnsw + cosine ops; UNIQUE
(kb_name, document_ref, ordinal) for upsert semantics."
```

---

## Task 4: kb_chunks_repo — pgvector Upsert + Search

**Files:**
- Create: `src/services/api/internal/data/kb_chunks_repo.go`
- Create: `src/services/api/internal/data/kb_chunks_repo_integration_test.go`

**Steps:**

- [ ] **Step 1: Write integration test (skips if integration env not set)**

Create `src/services/api/internal/data/kb_chunks_repo_integration_test.go`:

```go
//go:build !desktop

package data

import (
	"context"
	"math"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
)

func setupKBChunksRepo(t *testing.T) (*KBChunksRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_chunks")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	repo, err := NewKBChunksRepository(pool)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo, ctx
}

// makeVec returns a normalized fake vector of dim D where slot `pos`
// is 1.0 and all other slots are 0. Cosine similarity between two
// such vectors is 1.0 when pos matches, 0.0 otherwise.
func makeVec(dim, pos int) []float32 {
	v := make([]float32, dim)
	v[pos] = 1.0
	return v
}

func TestKBChunksUpsertAndSearch(t *testing.T) {
	repo, ctx := setupKBChunksRepo(t)
	dim := repo.Dim()
	if dim <= 0 {
		t.Fatalf("repo.Dim()=%d, expected >0", dim)
	}
	in := []KBChunkUpsert{
		{KBName: "physics", DocumentRef: "doc-1", Ordinal: 0, Text: "光的干涉…", TokenCount: 42, Embedding: makeVec(dim, 0)},
		{KBName: "physics", DocumentRef: "doc-1", Ordinal: 1, Text: "电磁感应…", TokenCount: 51, Embedding: makeVec(dim, 1)},
		{KBName: "physics", DocumentRef: "doc-2", Ordinal: 0, Text: "热力学第一定律…", TokenCount: 38, Embedding: makeVec(dim, 2)},
	}
	if err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Query closest to slot 1 → expect chunk (doc-1, ordinal 1) first.
	hits, err := repo.Search(ctx, "physics", makeVec(dim, 1), 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].DocumentRef != "doc-1" || hits[0].Ordinal != 1 {
		t.Errorf("top hit: got (%s,%d), want (doc-1,1)", hits[0].DocumentRef, hits[0].Ordinal)
	}
	if math.Abs(float64(hits[0].Score-1.0)) > 0.01 {
		t.Errorf("top score should be ~1.0 (cosine), got %f", hits[0].Score)
	}
}

func TestKBChunksUpsertIsIdempotent(t *testing.T) {
	repo, ctx := setupKBChunksRepo(t)
	dim := repo.Dim()
	row := KBChunkUpsert{KBName: "k", DocumentRef: "d", Ordinal: 0, Text: "first", TokenCount: 10, Embedding: makeVec(dim, 0)}
	if err := repo.Upsert(ctx, []KBChunkUpsert{row}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	row.Text = "updated"
	if err := repo.Upsert(ctx, []KBChunkUpsert{row}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	hits, _ := repo.Search(ctx, "k", makeVec(dim, 0), 5)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Text != "updated" {
		t.Errorf("expected updated text, got %q", hits[0].Text)
	}
}

func TestKBChunksSearchIsolatesByKBName(t *testing.T) {
	repo, ctx := setupKBChunksRepo(t)
	dim := repo.Dim()
	_ = repo.Upsert(ctx, []KBChunkUpsert{
		{KBName: "kb-a", DocumentRef: "d", Ordinal: 0, Text: "in a", TokenCount: 1, Embedding: makeVec(dim, 0)},
		{KBName: "kb-b", DocumentRef: "d", Ordinal: 0, Text: "in b", TokenCount: 1, Embedding: makeVec(dim, 0)},
	})
	hits, _ := repo.Search(ctx, "kb-a", makeVec(dim, 0), 5)
	if len(hits) != 1 || hits[0].Text != "in a" {
		t.Fatalf("kb isolation broken: %+v", hits)
	}
}
```

- [ ] **Step 2: Run, expect compile failure (KBChunksRepository undefined)**

```bash
cd src/services/api
go build ./...
```

Expected: `undefined: KBChunksRepository`.

- [ ] **Step 3: Implement the repo**

Create `src/services/api/internal/data/kb_chunks_repo.go`:

```go
package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KBChunksRepository persists and searches embedded chunks via pgvector.
// M0 uses a single flat table keyed by (kb_name, document_ref, ordinal);
// M1 will replace this with FK-bearing knowledge_bases / kb_documents.
type KBChunksRepository struct {
	pool DB
	dim  int
}

// KBChunkUpsert is the input row for Upsert.
type KBChunkUpsert struct {
	KBName      string
	DocumentRef string
	Ordinal     int
	Text        string
	TokenCount  int
	Embedding   []float32
}

// KBChunkHit is a search result.
type KBChunkHit struct {
	ID          uuid.UUID
	KBName      string
	DocumentRef string
	Ordinal     int
	Text        string
	TokenCount  int
	Score       float32 // cosine similarity in [-1, 1]; 1 = identical
}

// NewKBChunksRepository probes the pgvector column dimension once and caches it.
// This makes Dim() cheap and exposes mismatches early (e.g. forgot to migrate).
func NewKBChunksRepository(pool DB) (*KBChunksRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	var dim int
	// information_schema.columns + pg_attribute → pgvector typmod is dim<<16 | dim.
	// Simpler: probe by selecting an empty vector then reading via vector_dims.
	// But the table may be empty, so query the catalog.
	row := pool.QueryRow(context.Background(), `
SELECT a.atttypmod
FROM   pg_attribute a
JOIN   pg_class c ON c.oid = a.attrelid
WHERE  c.relname = 'kb_chunks' AND a.attname = 'embedding'`)
	if err := row.Scan(&dim); err != nil {
		return nil, fmt.Errorf("probe pgvector dim: %w", err)
	}
	if dim <= 0 {
		return nil, fmt.Errorf("invalid pgvector dim from catalog: %d (run migration 00192?)", dim)
	}
	return &KBChunksRepository{pool: pool, dim: dim}, nil
}

// Dim returns the pgvector column dimension. Always call this before
// constructing Embedder configs to verify they agree.
func (r *KBChunksRepository) Dim() int { return r.dim }

// Upsert writes chunks. Conflict on (kb_name, document_ref, ordinal) is an UPDATE.
func (r *KBChunksRepository) Upsert(ctx context.Context, rows []KBChunkUpsert) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, row := range rows {
		if len(row.Embedding) != r.dim {
			return fmt.Errorf("row (kb=%s,doc=%s,ord=%d): embedding dim %d != table dim %d",
				row.KBName, row.DocumentRef, row.Ordinal, len(row.Embedding), r.dim)
		}
		batch.Queue(`
INSERT INTO kb_chunks (kb_name, document_ref, ordinal, text, token_count, embedding)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (kb_name, document_ref, ordinal)
DO UPDATE SET text = EXCLUDED.text,
              token_count = EXCLUDED.token_count,
              embedding = EXCLUDED.embedding`,
			row.KBName, row.DocumentRef, row.Ordinal, row.Text, row.TokenCount, vecLiteral(row.Embedding))
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("upsert row %d: %w", i, err)
		}
	}
	return nil
}

// Search returns up to k chunks in kbName ordered by cosine similarity desc.
func (r *KBChunksRepository) Search(ctx context.Context, kbName string, query []float32, k int) ([]KBChunkHit, error) {
	if len(query) != r.dim {
		return nil, fmt.Errorf("query dim %d != table dim %d", len(query), r.dim)
	}
	if k <= 0 {
		k = 8
	}
	// Cosine distance via <=>; similarity = 1 - distance.
	rows, err := r.pool.Query(ctx, `
SELECT id, kb_name, document_ref, ordinal, text, token_count,
       1 - (embedding <=> $2) AS score
FROM   kb_chunks
WHERE  kb_name = $1
ORDER  BY embedding <=> $2
LIMIT  $3`,
		kbName, vecLiteral(query), k)
	if err != nil {
		return nil, fmt.Errorf("kb search: %w", err)
	}
	defer rows.Close()
	var out []KBChunkHit
	for rows.Next() {
		var h KBChunkHit
		if err := rows.Scan(&h.ID, &h.KBName, &h.DocumentRef, &h.Ordinal, &h.Text, &h.TokenCount, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// vecLiteral renders a []float32 as pgvector's text representation,
// e.g. "[0.1,0.2,0.3]". pgx encodes this string into the vector column.
func vecLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 6)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}
```

- [ ] **Step 4: Run integration tests with the env flag**

```bash
# Start pgvector pg16 (if not already running for Task 3)
docker run --rm -d --name pgv-test -e POSTGRES_PASSWORD=test -p 5599:5432 pgvector/pgvector:pg16
sleep 3
cd src/services/api
ARKLOOP_RUN_INTEGRATION_TESTS=1 \
ARKLOOP_TEST_DATABASE_DSN=postgres://postgres:test@localhost:5599/postgres?sslmode=disable \
  go test ./internal/data/ -run KBChunks -v
docker stop pgv-test
```

Expected: 3 tests PASS (`TestKBChunksUpsertAndSearch`, `TestKBChunksUpsertIsIdempotent`, `TestKBChunksSearchIsolatesByKBName`).

- [ ] **Step 5: Commit**

```bash
git add src/services/api/internal/data/kb_chunks_repo.go \
        src/services/api/internal/data/kb_chunks_repo_integration_test.go
git commit -m "feat(api): add kb_chunks_repo with pgvector upsert/search

NewKBChunksRepository probes the column dim from the pg catalog and
caches it; Upsert does batched ON CONFLICT (kb_name, document_ref,
ordinal) DO UPDATE; Search uses <=> (cosine distance) and returns
similarity = 1 - distance. Integration tests cover happy-path,
idempotency, and kb_name isolation."
```

---

## Task 5: debug-token middleware + KB debug handlers

**Files:**
- Create: `src/services/api/internal/http/kbdebugapi/middleware.go`
- Create: `src/services/api/internal/http/kbdebugapi/handler.go`
- Create: `src/services/api/internal/http/kbdebugapi/middleware_test.go`
- Create: `src/services/api/internal/http/kbdebugapi/handler_test.go`

**Steps:**

- [ ] **Step 1: Write middleware test**

Create `src/services/api/internal/http/kbdebugapi/middleware_test.go`:

```go
package kbdebugapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireDebugTokenAcceptsMatchingBearer(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := RequireDebugToken("secret")(inner)
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("got %d, want 204", w.Code)
	}
}

func TestRequireDebugTokenRejectsWrongToken(t *testing.T) {
	h := RequireDebugToken("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not run") }))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("got %d, want 401", w.Code)
	}
}

func TestRequireDebugTokenRejectsMissingHeader(t *testing.T) {
	h := RequireDebugToken("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not run") }))
	req := httptest.NewRequest("GET", "/x", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("got %d, want 401", w.Code)
	}
}

func TestRequireDebugTokenRejectsWhenSecretEmpty(t *testing.T) {
	// If configured secret is empty, treat as disabled and reject all.
	h := RequireDebugToken("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("must not run") }))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Errorf("got %d, want 401", w.Code)
	}
}
```

- [ ] **Step 2: Implement middleware**

Create `src/services/api/internal/http/kbdebugapi/middleware.go`:

```go
// Package kbdebugapi hosts /v1/_debug/kb/* endpoints used only during M0
// to verify the chunker → embedder → pgvector pipeline. These routes
// are not exposed via the Gateway; they require a static Bearer token
// from env ARKLOOP_DEBUG_TOKEN (configured at handler wiring time).
//
// M1 retires this package and replaces it with kbapi behind workspace auth.
package kbdebugapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireDebugToken returns middleware that enforces Authorization: Bearer <token>
// with a constant-time compare. An empty configured token disables the route
// entirely (every request 401s) so misconfiguration fails closed.
func RequireDebugToken(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				http.Error(w, "kb debug routes disabled", http.StatusUnauthorized)
				return
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "bearer token required", http.StatusUnauthorized)
				return
			}
			provided := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				http.Error(w, "invalid debug token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 3: Run middleware test, expect pass**

```bash
cd src/services/api
go test ./internal/http/kbdebugapi/... -v
```

Expected: 4 tests PASS.

- [ ] **Step 4: Write handler test**

Create `src/services/api/internal/http/kbdebugapi/handler_test.go`:

```go
package kbdebugapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeIngester / fakeSearcher implement Ingester / Searcher for handler
// unit tests. The real implementations are wired in handler.go's
// constructor and exercised by Task 8's end-to-end integration test.

type fakeIngester struct {
	called    bool
	lastPath  string
	lastKBName string
	chunkCount int
}

func (f *fakeIngester) Ingest(ctx context.Context, filePath, kbName string) (int, error) {
	f.called = true
	f.lastPath = filePath
	f.lastKBName = kbName
	return f.chunkCount, nil
}

type fakeSearcher struct {
	hits []SearchHit
}

func (f *fakeSearcher) Search(ctx context.Context, kbName, query string, k int) ([]SearchHit, error) {
	return f.hits, nil
}

func TestIngestHandlerHappyPath(t *testing.T) {
	tmp := t.TempDir()
	textFile := filepath.Join(tmp, "in.txt")
	_ = os.WriteFile(textFile, []byte("hello world"), 0644)
	ing := &fakeIngester{chunkCount: 3}
	h := newIngestHandler(ing)
	body := strings.NewReader(`{"file_path":"` + textFile + `","kb_name":"k"}`)
	req := httptest.NewRequest("POST", "/v1/_debug/kb/ingest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		ChunkCount int `json:"chunk_count"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.ChunkCount != 3 {
		t.Errorf("chunk_count: got %d, want 3", resp.ChunkCount)
	}
	if !ing.called || ing.lastKBName != "k" {
		t.Errorf("ingester not invoked correctly: %+v", ing)
	}
}

func TestIngestHandlerRejectsMissingFields(t *testing.T) {
	h := newIngestHandler(&fakeIngester{})
	req := httptest.NewRequest("POST", "/v1/_debug/kb/ingest", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("got %d, want 400", w.Code)
	}
}

func TestSearchHandlerHappyPath(t *testing.T) {
	srch := &fakeSearcher{hits: []SearchHit{{DocumentRef: "d", Ordinal: 0, Text: "光的干涉…", Score: 0.97}}}
	h := newSearchHandler(srch)
	body := strings.NewReader(`{"kb_name":"k","query":"光","k":3}`)
	req := httptest.NewRequest("POST", "/v1/_debug/kb/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hits []SearchHit `json:"hits"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Hits) != 1 || resp.Hits[0].DocumentRef != "d" {
		t.Errorf("unexpected hits: %+v", resp.Hits)
	}
}
```

- [ ] **Step 5: Implement handlers**

Create `src/services/api/internal/http/kbdebugapi/handler.go`:

```go
package kbdebugapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// Ingester is implemented in package internal by composing bookchunker +
// embedding.Embedder + data.KBChunksRepository. Kept as an interface here
// to make handler tests free of those dependencies.
type Ingester interface {
	Ingest(ctx context.Context, filePath, kbName string) (chunkCount int, err error)
}

// Searcher is the read side of the same composition.
type Searcher interface {
	Search(ctx context.Context, kbName, query string, k int) ([]SearchHit, error)
}

// SearchHit is the JSON shape returned to callers.
type SearchHit struct {
	DocumentRef string  `json:"document_ref"`
	Ordinal     int     `json:"ordinal"`
	Text        string  `json:"text"`
	Score       float32 `json:"score"`
}

type ingestRequest struct {
	FilePath string `json:"file_path"`
	KBName   string `json:"kb_name"`
}

type ingestResponse struct {
	ChunkCount int   `json:"chunk_count"`
	DurationMS int64 `json:"duration_ms"`
}

type searchRequest struct {
	KBName string `json:"kb_name"`
	Query  string `json:"query"`
	K      int    `json:"k"`
}

type searchResponse struct {
	Hits []SearchHit `json:"hits"`
}

func newIngestHandler(ing Ingester) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ingestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.FilePath == "" || req.KBName == "" {
			http.Error(w, "file_path and kb_name required", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(req.FilePath); err != nil {
			http.Error(w, "file not accessible: "+err.Error(), http.StatusBadRequest)
			return
		}
		start := time.Now()
		count, err := ing.Ingest(r.Context(), req.FilePath, req.KBName)
		if err != nil {
			http.Error(w, "ingest failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingestResponse{
			ChunkCount: count,
			DurationMS: time.Since(start).Milliseconds(),
		})
	})
}

func newSearchHandler(srch Searcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.KBName == "" || req.Query == "" {
			http.Error(w, "kb_name and query required", http.StatusBadRequest)
			return
		}
		if req.K <= 0 || req.K > 50 {
			req.K = 8
		}
		hits, err := srch.Search(r.Context(), req.KBName, req.Query, req.K)
		if err != nil {
			http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResponse{Hits: hits})
	})
}

// Register wires both endpoints onto the supplied mux behind the debug-token
// middleware. mux must be an *http.ServeMux compatible router; pass the api's
// existing router. Routes:
//
//	POST /v1/_debug/kb/ingest
//	POST /v1/_debug/kb/search
func Register(mux *http.ServeMux, debugToken string, ing Ingester, srch Searcher) {
	guard := RequireDebugToken(debugToken)
	mux.Handle("POST /v1/_debug/kb/ingest", guard(newIngestHandler(ing)))
	mux.Handle("POST /v1/_debug/kb/search", guard(newSearchHandler(srch)))
}
```

- [ ] **Step 6: Run handler tests, expect pass**

```bash
cd src/services/api
go test ./internal/http/kbdebugapi/... -v
```

Expected: all 7 tests PASS (4 middleware + 3 handler).

- [ ] **Step 7: Commit**

```bash
git add src/services/api/internal/http/kbdebugapi
git commit -m "feat(api): add kbdebugapi pkg with token-gated ingest/search

Bearer-token middleware (constant-time compare, fail-closed on empty
secret) plus two POST handlers wired through Ingester/Searcher
interfaces. Routes will be registered on the api mux in the next
task, which provides concrete implementations from bookchunker +
embedding + data.KBChunksRepository."
```

---

## Task 6: Wire Ingester / Searcher concrete impls + register routes

**Files:**
- Create: `src/services/api/internal/kbingest/service.go` (composes chunker + embedder + repo)
- Modify: `src/services/api/internal/http/handler.go` (call kbdebugapi.Register)
- Modify: `src/services/api/internal/app/config.go` (read ARKLOOP_DEBUG_TOKEN + Doubao embedding settings)
- Modify: `src/services/api/internal/app/app.go` (construct kbingest.Service, pass into handler)

**Steps:**

- [ ] **Step 1: Implement the composing service**

Create `src/services/api/internal/kbingest/service.go`:

```go
// Package kbingest composes bookchunker + embedding + data.KBChunksRepository
// into the M0 Ingester/Searcher used by the kb-debug HTTP handlers. M1 will
// move this composition into a worker job; the interfaces stay stable so the
// shared packages don't need to change.
package kbingest

import (
	"context"
	"fmt"
	"os"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/http/kbdebugapi"
	"arkloop/services/shared/bookchunker"
	"arkloop/services/shared/embedding"
)

// Service implements kbdebugapi.Ingester and kbdebugapi.Searcher.
type Service struct {
	embedder embedding.Embedder
	repo     *data.KBChunksRepository
}

// New constructs a Service. The embedder's Dim() must match repo.Dim() or
// New returns an error — this is the M0 cross-check that S0 fed the right
// number into both halves of the pipeline.
func New(embedder embedding.Embedder, repo *data.KBChunksRepository) (*Service, error) {
	if embedder.Dim() != repo.Dim() {
		return nil, fmt.Errorf("kbingest: embedder dim %d != repo dim %d (S0 mismatch?)",
			embedder.Dim(), repo.Dim())
	}
	return &Service{embedder: embedder, repo: repo}, nil
}

// Ingest reads filePath, chunks it, embeds, and upserts under kbName.
// document_ref is taken as the file basename.
func (s *Service) Ingest(ctx context.Context, filePath, kbName string) (int, error) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}
	chunks, err := bookchunker.Chunk(string(buf), bookchunker.DefaultOptions())
	if err != nil {
		return 0, fmt.Errorf("chunk: %w", err)
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	vecs, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}
	docRef := baseName(filePath)
	rows := make([]data.KBChunkUpsert, len(chunks))
	for i, c := range chunks {
		rows[i] = data.KBChunkUpsert{
			KBName: kbName, DocumentRef: docRef, Ordinal: c.Ordinal,
			Text: c.Text, TokenCount: c.TokenCount, Embedding: vecs[i],
		}
	}
	if err := s.repo.Upsert(ctx, rows); err != nil {
		return 0, fmt.Errorf("upsert: %w", err)
	}
	return len(chunks), nil
}

// Search runs a single-query retrieval. Embeds the query, then asks the repo.
func (s *Service) Search(ctx context.Context, kbName, query string, k int) ([]kbdebugapi.SearchHit, error) {
	vecs, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	hits, err := s.repo.Search(ctx, kbName, vecs[0], k)
	if err != nil {
		return nil, fmt.Errorf("repo search: %w", err)
	}
	out := make([]kbdebugapi.SearchHit, len(hits))
	for i, h := range hits {
		out[i] = kbdebugapi.SearchHit{
			DocumentRef: h.DocumentRef, Ordinal: h.Ordinal,
			Text: h.Text, Score: h.Score,
		}
	}
	return out, nil
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
```

- [ ] **Step 2: Add config fields and env loading**

Open `src/services/api/internal/app/config.go`. Locate the `Config` struct (top of file) and append these fields at the end of the struct definition:

```go
// KB debug routes (M0 only; M1 retires these).
KBDebugToken          string  // env ARKLOOP_DEBUG_TOKEN — required to enable routes
DoubaoEmbedAPIKey     string  // env ARK_API_KEY
DoubaoEmbedBaseURL    string  // env ARK_BASE_URL, default https://ark.cn-beijing.volces.com/api/v3
DoubaoEmbedModel      string  // env ARK_EMBED_MODEL, default doubao-embedding-text-240715
DoubaoEmbedBatchSize  int     // env ARK_EMBED_BATCH, default 32
```

In the same file, locate the `Load` (or `LoadFromEnv`, whichever the file defines — search for `os.Getenv(` to find the existing loader). At the end of that function, before `return cfg, nil`, append:

```go
cfg.KBDebugToken = os.Getenv("ARKLOOP_DEBUG_TOKEN")
cfg.DoubaoEmbedAPIKey = os.Getenv("ARK_API_KEY")
cfg.DoubaoEmbedBaseURL = os.Getenv("ARK_BASE_URL")
if cfg.DoubaoEmbedBaseURL == "" {
	cfg.DoubaoEmbedBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
}
cfg.DoubaoEmbedModel = os.Getenv("ARK_EMBED_MODEL")
if cfg.DoubaoEmbedModel == "" {
	cfg.DoubaoEmbedModel = "doubao-embedding-text-240715"
}
if v := os.Getenv("ARK_EMBED_BATCH"); v != "" {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		cfg.DoubaoEmbedBatchSize = n
	}
}
if cfg.DoubaoEmbedBatchSize == 0 {
	cfg.DoubaoEmbedBatchSize = 32
}
```

If `os` / `strconv` are not yet imported in this file, add them.

- [ ] **Step 3: Construct and wire the service in `app.go`**

Open `src/services/api/internal/app/app.go`. Two changes:

**(a) Add imports** (top of file, in the existing import block):

```go
"arkloop/services/api/internal/kbingest"
"arkloop/services/shared/embedding"
```

**(b) Build the service** — locate the main `Run` (or `Start`) function. Find the call that constructs the HTTP handler (search for `http.NewHandler` or `http.NewServer` — it takes a `HandlerConfig`). Immediately **before** that handler construction, insert:

```go
// Optional: kb-debug routes for M0 verification of the KB pipeline.
// Service is built only when both ARKLOOP_DEBUG_TOKEN and ARK_API_KEY are set;
// otherwise nil is passed and the routes return 401 fail-closed.
var kbIngestService *kbingest.Service
if cfg.KBDebugToken != "" && cfg.DoubaoEmbedAPIKey != "" {
	kbRepo, err := data.NewKBChunksRepository(pool)
	if err != nil {
		return fmt.Errorf("kb_chunks repo: %w", err)
	}
	doubao := embedding.NewDoubao(embedding.DoubaoConfig{
		BaseURL:    cfg.DoubaoEmbedBaseURL,
		APIKey:     cfg.DoubaoEmbedAPIKey,
		Model:      cfg.DoubaoEmbedModel,
		BatchSize:  cfg.DoubaoEmbedBatchSize,
		MaxRetries: 3,
		Dim:        kbRepo.Dim(),
	})
	kbIngestService, err = kbingest.New(doubao, kbRepo)
	if err != nil {
		return fmt.Errorf("kbingest: %w", err)
	}
}
```

**(c) Pass into `HandlerConfig{}`** — in the `HandlerConfig{...}` literal that follows, add these two field assignments at the bottom of the literal (after the existing fields, before the closing brace):

```go
KBIngestService: kbIngestService,
KBDebugToken:    cfg.KBDebugToken,
```

- [ ] **Step 4: Extend HandlerConfig + register routes**

Open `src/services/api/internal/http/handler.go`.

**(a) Add imports** to the existing import block:

```go
"arkloop/services/api/internal/http/kbdebugapi"
"arkloop/services/api/internal/kbingest"
```

**(b) Extend `HandlerConfig`** — locate the `HandlerConfig` struct (defined around line 60). Add at the end of the struct (before the closing brace):

```go
// Optional M0 kb-debug routes (nil disables them).
KBIngestService *kbingest.Service
KBDebugToken    string
```

**(c) Register the routes** — inside the function that builds the mux (search the file for `mux.Handle(` or `mux := http.NewServeMux()`). Find the block where other endpoints are registered. At the **bottom** of that block (after all other `mux.Handle` calls, before any final `return`), add:

```go
// M0: book-kb-rag debug pipeline. Skipped entirely when not wired.
if cfg.KBIngestService != nil {
	kbdebugapi.Register(mux, cfg.KBDebugToken, cfg.KBIngestService, cfg.KBIngestService)
}
```

- [ ] **Step 5: Build the api binary, expect green**

```bash
cd src/services/api
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Quick sanity smoke (handler-level, no DB)**

```bash
cd src/services/api
go test ./... -run "KBDebug\|KBChunks" 2>&1 | tail -20
```

Expected: handler/middleware tests still PASS; KBChunks integration tests are gated by env (will skip without `ARKLOOP_RUN_INTEGRATION_TESTS`).

- [ ] **Step 7: Commit**

```bash
git add src/services/api/internal/kbingest \
        src/services/api/internal/app/config.go \
        src/services/api/internal/app/app.go \
        src/services/api/internal/http/handler.go
git commit -m "feat(api): compose kbingest service and register debug routes

Wire bookchunker + Doubao Embedder + KBChunksRepository into the
kbingest.Service that implements kbdebugapi's Ingester/Searcher.
Service is built only when ARKLOOP_DEBUG_TOKEN and ARK_API_KEY are
both set (M0 verification opt-in)."
```

---

## Task 7: compose.yaml + deploy script — pgvector image swap

**Files:**
- Modify: `compose.yaml:3`
- Modify: `setup.sh` (or `scripts/deploy-source-to-server.sh` — pick whichever runs `docker compose up -d postgres`)

**Steps:**

- [ ] **Step 1: Read current compose.yaml postgres block**

```bash
grep -n "postgres:" compose.yaml | head -5
sed -n '1,20p' compose.yaml
```

Identify the line `image: postgres:16-alpine`.

- [ ] **Step 2: Swap the image**

Edit `compose.yaml`, change line 3 from:
```yaml
    image: postgres:16-alpine
```
to:
```yaml
    image: pgvector/pgvector:pg16
```

> **Why this image:** `pgvector/pgvector:pg16` is the official pgvector-bundled postgres 16 image. It is a drop-in replacement for `postgres:16-alpine` (compatible data directory, same env vars). Existing volumes upgrade in place because the underlying postgres binary is the same minor; only the `vector` extension binary is added.

- [ ] **Step 3: Add a startup smoke to the postgres healthcheck (optional but cheap)**

Find the existing healthcheck in the postgres service block (search for `healthcheck:` under `postgres:`). If it uses `pg_isready`, leave alone. Otherwise no change needed — the migration in Task 3 already runs `CREATE EXTENSION IF NOT EXISTS vector`, which fails loudly if the extension is missing.

- [ ] **Step 4: Add an upgrade note in setup.sh**

Open `setup.sh`. Find where it brings up postgres (search for `docker compose up`). Add an inline note above that line:

```bash
# NOTE: 自 M0 of book-kb-rag 起，postgres 镜像由 postgres:16-alpine 改为
# pgvector/pgvector:pg16。已有部署升级方式：docker compose pull postgres
# 后 docker compose up -d postgres；数据卷无需迁移（同一 pg 16 minor）。
```

- [ ] **Step 5: Verify locally**

```bash
docker compose pull postgres
docker compose up -d postgres
sleep 5
docker compose exec postgres psql -U $ARKLOOP_POSTGRES_USER -d $ARKLOOP_POSTGRES_DB -c "CREATE EXTENSION IF NOT EXISTS vector; SELECT extname, extversion FROM pg_extension WHERE extname='vector';"
```

Expected: output row `vector | 0.x.y`.

If you have an existing dev volume with the old image: `docker compose down postgres && docker compose up -d postgres`. Confirm migrations still apply: `cd src/services/api && go run ./cmd/migrate up`.

- [ ] **Step 6: Commit**

```bash
git add compose.yaml setup.sh
git commit -m "chore(compose): swap postgres image to pgvector/pgvector:pg16

Required by M0 of book-kb-rag (CREATE EXTENSION vector). Drop-in
replacement for postgres:16-alpine — same pg 16 minor, same data
directory, no volume migration needed."
```

---

## Task 8: End-to-end integration test — ingest → search

**Files:**
- Create: `src/services/api/internal/kbingest/e2e_integration_test.go`
- Reuse: `src/services/shared/bookchunker/testdata/cn_textbook_excerpt.txt` (created in Task 1 Step 6)

**Steps:**

- [ ] **Step 1: Write the test**

Create `src/services/api/internal/kbingest/e2e_integration_test.go`:

```go
//go:build !desktop

package kbingest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/kbingest"
	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
	"arkloop/services/shared/embedding"
)

// TestKBPipelineE2E exercises chunker + Doubao Embedder + pgvector search.
// Skipped unless ARKLOOP_RUN_INTEGRATION_TESTS=1 AND ARK_API_KEY is set.
// Requires a pgvector-enabled postgres reachable via ARKLOOP_TEST_DATABASE_DSN.
func TestKBPipelineE2E(t *testing.T) {
	if os.Getenv("ARK_API_KEY") == "" {
		t.Skip("ARK_API_KEY required; skipping live Doubao E2E test")
	}
	db := testutil.SetupPostgresDatabase(t, "kbingest_e2e")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 4})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo, err := data.NewKBChunksRepository(pool)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	doubao := embedding.NewDoubao(embedding.DoubaoConfig{
		BaseURL:    "https://ark.cn-beijing.volces.com/api/v3",
		APIKey:     os.Getenv("ARK_API_KEY"),
		Model:      "doubao-embedding-text-240715",
		BatchSize:  16,
		MaxRetries: 3,
		Dim:        repo.Dim(),
	})
	svc, err := kbingest.New(doubao, repo)
	if err != nil {
		t.Fatalf("kbingest: %v", err)
	}

	// Locate fixture: testdata is in shared/bookchunker; we walk up.
	fixture := findFixture(t)
	count, err := svc.Ingest(ctx, fixture, "e2e-physics")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if count == 0 {
		t.Fatal("ingest produced 0 chunks")
	}
	t.Logf("ingested %d chunks", count)

	hits, err := svc.Search(ctx, "e2e-physics", "光的干涉", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("search returned 0 hits")
	}
	if hits[0].Score < 0.4 {
		t.Errorf("top score too low: %f (fixture may not contain related text)", hits[0].Score)
	}
	if !strings.Contains(hits[0].Text, "光") {
		t.Errorf("top hit text doesn't mention 光: %q", hits[0].Text)
	}
}

func findFixture(t *testing.T) string {
	// Walk up looking for src/services/shared/bookchunker/testdata
	wd, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(wd, "..", "..", "..", "..", "shared", "bookchunker", "testdata", "cn_textbook_excerpt.txt")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		wd = filepath.Dir(wd)
	}
	t.Fatal("could not locate cn_textbook_excerpt.txt fixture")
	return ""
}
```

- [ ] **Step 2: Run it against real Doubao + pgvector**

```bash
# Bring up pgvector
docker run --rm -d --name pgv-e2e -e POSTGRES_PASSWORD=test -p 5599:5432 pgvector/pgvector:pg16
sleep 3

cd src/services/api
ARKLOOP_RUN_INTEGRATION_TESTS=1 \
ARKLOOP_TEST_DATABASE_DSN=postgres://postgres:test@localhost:5599/postgres?sslmode=disable \
ARK_API_KEY=<your doubao key> \
  go test ./internal/kbingest/... -run TestKBPipelineE2E -v -timeout 120s

docker stop pgv-e2e
```

Expected: `PASS` with log lines like `ingested 3 chunks` and a top score > 0.4. If score is < 0.4, the fixture content doesn't sufficiently relate to "光的干涉" — replace the fixture with something more on-topic.

- [ ] **Step 3: Smoke-test the HTTP route end-to-end (manual)**

Start the api locally with all env wired, then:

```bash
export DEBUG_TOKEN=$(uuidgen)
# Set ARKLOOP_DEBUG_TOKEN=$DEBUG_TOKEN and ARK_API_KEY=<key> in your .env

# Ingest
curl -s -X POST http://localhost:19001/v1/_debug/kb/ingest \
  -H "Authorization: Bearer $DEBUG_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"file_path":"/abs/path/to/cn_textbook_excerpt.txt","kb_name":"smoke"}'
# Expected: {"chunk_count":N,"duration_ms":XXX}

# Search
curl -s -X POST http://localhost:19001/v1/_debug/kb/search \
  -H "Authorization: Bearer $DEBUG_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"kb_name":"smoke","query":"光的干涉","k":3}'
# Expected: {"hits":[{"document_ref":"cn_textbook_excerpt.txt","ordinal":0,"text":"光的干涉…","score":0.9...}]}
```

This is the M0 acceptance criterion.

- [ ] **Step 4: Commit**

```bash
git add src/services/api/internal/kbingest/e2e_integration_test.go
git commit -m "test(api): add KB pipeline E2E integration test

Exercises bookchunker + real Doubao Ark embeddings + pgvector search
through the kbingest.Service. Gated by ARK_API_KEY and
ARKLOOP_RUN_INTEGRATION_TESTS so it doesn't run in unit CI."
```

---

## Final M0 Verification Checklist

After all tasks committed, run these once more before considering M0 done:

- [ ] `cd src/services/shared && go test ./bookchunker/... ./embedding/... -v` → all PASS
- [ ] `cd src/services/api && go build ./...` → no errors
- [ ] With pgvector + ARK_API_KEY: full E2E test (Task 8 Step 2) → PASS
- [ ] Manual curl smoke (Task 8 Step 3) → both requests return expected JSON
- [ ] Design doc's "已锁决策" table reflects the actual S0 dimension
- [ ] Migration `00192_kb_chunks.sql` is the latest in `migrations/`
- [ ] `compose.yaml` shows `pgvector/pgvector:pg16`

## What M0 Explicitly Does Not Address

These are not failures of M0; they are M1 work:

- KB / document REST CRUD (no `POST /v1/knowledge-bases`)
- PDF / DOCX parsing
- Workspace auth on the routes
- Persona, UI, QuestionStore
- Job queue / async ingestion
- Tracking ingestion status per document
- Multi-tenancy via `account_id`
- Filtering search by `document_ref` etc.
- Re-embedding / chunk versioning
- LLM-based question generation

All of those land in M1, which begins with Spike S1 (PDF parsing) and Spike S2 (exam contract).
