package kbingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"arkloop/services/shared/bookparser"
)

type fakeEmbedder struct {
	dim       int
	lastTexts []string
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	f.lastTexts = append([]string(nil), texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, f.dim)
		vec[0] = float32(i + 1)
		out[i] = vec
	}
	return out, nil
}

func (f *fakeEmbedder) Dim() int { return f.dim }

func TestServiceIngestParsesChunksEmbedsThenRefusesM0Schema(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "physics.txt")
	if err := os.WriteFile(path, []byte("光的干涉是波动光学的重要内容。"), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := &fakeEmbedder{dim: 4}
	svc := &Service{embedder: emb, parser: bookparser.NewTextOnlyParser()}

	count, err := svc.Ingest(context.Background(), path, "kb-physics")
	if err == nil || !strings.Contains(err.Error(), "M0-only") {
		t.Fatalf("expected M0-only error, got count=%d err=%v", count, err)
	}
	if count != 0 {
		t.Fatalf("count=%d, want 0", count)
	}
	if len(emb.lastTexts) != 1 || emb.lastTexts[0] == "" {
		t.Fatalf("embed texts not captured: %+v", emb.lastTexts)
	}
}

func TestServiceSearchRefusesM0Schema(t *testing.T) {
	svc := &Service{}
	hits, err := svc.Search(context.Background(), "kb-physics", "光的干涉", 3)
	if err == nil || !strings.Contains(err.Error(), "M0-only") {
		t.Fatalf("expected M0-only error, got hits=%+v err=%v", hits, err)
	}
}
