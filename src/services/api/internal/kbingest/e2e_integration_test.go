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

// TestKBPipelineE2E exercises chunker + real Doubao embeddings + pgvector
// search. It is gated by ARKLOOP_RUN_INTEGRATION_TESTS and an Ark API key.
func TestKBPipelineE2E(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("ARK_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ARKLOOP_DOUBAO_API_KEY"))
	}
	if apiKey == "" {
		t.Skip("ARK_API_KEY or ARKLOOP_DOUBAO_API_KEY required; skipping live Doubao E2E test")
	}

	db := testutil.SetupPostgresDatabase(t, "kbingest_e2e")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
		BaseURL:    envOrDefault("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"),
		APIKey:     apiKey,
		Model:      envOrDefault("ARK_EMBED_MODEL", "doubao-embedding-text-240715"),
		BatchSize:  16,
		MaxRetries: 3,
		Dim:        repo.Dim(),
	})
	svc, err := kbingest.New(doubao, repo)
	if err != nil {
		t.Fatalf("kbingest: %v", err)
	}

	count, err := svc.Ingest(ctx, findFixture(t), "e2e-physics")
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
		t.Errorf("top score too low: %f", hits[0].Score)
	}
	if !strings.Contains(hits[0].Text, "光") {
		t.Errorf("top hit text does not mention 光: %q", hits[0].Text)
	}
}

func envOrDefault(key, fallback string) string {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		return raw
	}
	return fallback
}

func findFixture(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(wd, "src", "services", "shared", "bookchunker", "testdata", "cn_textbook_excerpt.txt")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		next := filepath.Dir(wd)
		if next == wd {
			break
		}
		wd = next
	}
	t.Fatal("could not locate cn_textbook_excerpt.txt fixture")
	return ""
}
