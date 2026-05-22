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
	"strings"
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
	batch := flag.Int("batch", 1, "number of duplicate inputs to send")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
	flag.Parse()

	apiKey := strings.TrimSpace(os.Getenv("ARK_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ARKLOOP_DOUBAO_API_KEY"))
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ARK_API_KEY or ARKLOOP_DOUBAO_API_KEY env required")
		os.Exit(2)
	}
	if *batch <= 0 {
		fmt.Fprintln(os.Stderr, "batch must be positive")
		os.Exit(2)
	}

	inputs := make([]string, *batch)
	for i := range inputs {
		inputs[i] = *input
	}
	body, _ := json.Marshal(embedReq{Model: *model, Input: inputs})
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(*baseURL, "/")+"/embeddings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
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
	fmt.Printf("model=%s dim=%d batch=%d latency_ms=%d prompt_tokens=%d total_tokens=%d\n",
		parsed.Model,
		len(parsed.Data[0].Embedding),
		len(parsed.Data),
		time.Since(start).Milliseconds(),
		parsed.Usage.PromptTokens,
		parsed.Usage.TotalTokens,
	)
}
