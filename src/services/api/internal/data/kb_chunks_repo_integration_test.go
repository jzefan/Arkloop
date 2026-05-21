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

// makeVec returns a normalized fake vector of dim D where slot pos is 1.0
// and all other slots are 0. Cosine similarity is 1.0 when pos matches, 0.0 otherwise.
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
		{KBName: "physics", DocumentRef: "doc-1", Ordinal: 0, Text: "光的干涉...", TokenCount: 42, Embedding: makeVec(dim, 0)},
		{KBName: "physics", DocumentRef: "doc-1", Ordinal: 1, Text: "电磁感应...", TokenCount: 51, Embedding: makeVec(dim, 1)},
		{KBName: "physics", DocumentRef: "doc-2", Ordinal: 0, Text: "热力学第一定律...", TokenCount: 38, Embedding: makeVec(dim, 2)},
	}
	if err := repo.Upsert(ctx, in); err != nil {
		t.Fatalf("upsert: %v", err)
	}

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
	hits, err := repo.Search(ctx, "k", makeVec(dim, 0), 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
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
	if err := repo.Upsert(ctx, []KBChunkUpsert{
		{KBName: "kb-a", DocumentRef: "d", Ordinal: 0, Text: "in a", TokenCount: 1, Embedding: makeVec(dim, 0)},
		{KBName: "kb-b", DocumentRef: "d", Ordinal: 0, Text: "in b", TokenCount: 1, Embedding: makeVec(dim, 0)},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	hits, err := repo.Search(ctx, "kb-a", makeVec(dim, 0), 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Text != "in a" {
		t.Fatalf("kb isolation broken: %+v", hits)
	}
}
