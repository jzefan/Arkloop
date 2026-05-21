package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DoubaoConfig configures the Doubao Ark embedding backend. Dim is the
// authoritative expected dimension; any response of a different size is
// rejected with ErrDimMismatch to guard pgvector(N).
type DoubaoConfig struct {
	BaseURL     string        // e.g. https://ark.cn-beijing.volces.com/api/v3
	APIKey      string        // Authorization: Bearer <APIKey>
	Model       string        // e.g. a Doubao embedding endpoint id
	BatchSize   int           // <= S0 probed max, usually 32
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
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
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
		vecs, err := d.embedBatch(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		copy(out[start:end], vecs)
	}
	return out, nil
}

func (d *Doubao) embedBatch(ctx context.Context, batch []string) ([][]float32, error) {
	body, _ := json.Marshal(arkReq{Model: d.cfg.Model, Input: batch})
	var lastErr error
	for attempt := 0; attempt <= d.cfg.MaxRetries; attempt++ {
		vecs, retry, err := d.tryEmbedBatch(ctx, body, len(batch))
		if err == nil {
			return vecs, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
		if attempt == d.cfg.MaxRetries {
			break
		}
		wait := d.cfg.BaseBackoff << attempt
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("%w: %v", ErrUpstream, lastErr)
}

func (d *Doubao) tryEmbedBatch(ctx context.Context, body []byte, want int) ([][]float32, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		retry := resp.StatusCode >= 500
		return nil, retry, fmt.Errorf("ark http %d: %s", resp.StatusCode, string(raw))
	}

	var parsed arkResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, false, fmt.Errorf("decode ark response: %w", err)
	}
	if len(parsed.Data) != want {
		return nil, false, fmt.Errorf("ark returned %d vectors, want %d", len(parsed.Data), want)
	}
	vecs := make([][]float32, want)
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= want {
			return nil, false, fmt.Errorf("ark returned out-of-range index %d", item.Index)
		}
		if len(item.Embedding) != d.cfg.Dim {
			return nil, false, fmt.Errorf("%w: got %d want %d", ErrDimMismatch, len(item.Embedding), d.cfg.Dim)
		}
		vecs[item.Index] = item.Embedding
	}
	for i, vec := range vecs {
		if vec == nil {
			return nil, false, fmt.Errorf("ark omitted vector for index %d", i)
		}
	}
	return vecs, false, nil
}
