package kbingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"arkloop/services/api/internal/data"
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

type fakeChunksRepo struct {
	dim            int
	upserts        []data.KBChunkUpsert
	lastSearchKB   string
	lastSearchVec  []float32
	lastSearchK    int
	searchResponse []data.KBChunkHit
}

func (f *fakeChunksRepo) Dim() int { return f.dim }

func (f *fakeChunksRepo) Upsert(ctx context.Context, rows []data.KBChunkUpsert) error {
	f.upserts = append([]data.KBChunkUpsert(nil), rows...)
	return nil
}

func (f *fakeChunksRepo) Search(ctx context.Context, kbName string, query []float32, k int) ([]data.KBChunkHit, error) {
	f.lastSearchKB = kbName
	f.lastSearchVec = append([]float32(nil), query...)
	f.lastSearchK = k
	return f.searchResponse, nil
}

func TestServiceIngestChunksEmbedsAndUpserts(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "physics.txt")
	if err := os.WriteFile(path, []byte("光的干涉是波动光学的重要内容。"), 0o644); err != nil {
		t.Fatal(err)
	}
	emb := &fakeEmbedder{dim: 4}
	repo := &fakeChunksRepo{dim: 4}
	svc, err := newWithRepository(emb, repo)
	if err != nil {
		t.Fatal(err)
	}

	count, err := svc.Ingest(context.Background(), path, "kb-physics")
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}
	if len(emb.lastTexts) != 1 || emb.lastTexts[0] == "" {
		t.Fatalf("embed texts not captured: %+v", emb.lastTexts)
	}
	if len(repo.upserts) != 1 {
		t.Fatalf("upserts=%d, want 1", len(repo.upserts))
	}
	row := repo.upserts[0]
	if row.KBName != "kb-physics" || row.DocumentRef != "physics.txt" || row.Ordinal != 0 {
		t.Fatalf("unexpected upsert row identity: %+v", row)
	}
	if len(row.Embedding) != 4 || row.Embedding[0] != 1 {
		t.Fatalf("unexpected embedding: %+v", row.Embedding)
	}
}

func TestServiceSearchEmbedsQueryAndMapsHits(t *testing.T) {
	emb := &fakeEmbedder{dim: 4}
	repo := &fakeChunksRepo{
		dim: 4,
		searchResponse: []data.KBChunkHit{{
			DocumentRef: "physics.txt",
			Ordinal:     2,
			Text:        "干涉条纹",
			Score:       0.91,
		}},
	}
	svc, err := newWithRepository(emb, repo)
	if err != nil {
		t.Fatal(err)
	}

	hits, err := svc.Search(context.Background(), "kb-physics", "光的干涉", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(emb.lastTexts) != 1 || emb.lastTexts[0] != "光的干涉" {
		t.Fatalf("query was not embedded correctly: %+v", emb.lastTexts)
	}
	if repo.lastSearchKB != "kb-physics" || repo.lastSearchK != 3 || len(repo.lastSearchVec) != 4 {
		t.Fatalf("repo search args wrong: kb=%q k=%d vec=%+v", repo.lastSearchKB, repo.lastSearchK, repo.lastSearchVec)
	}
	if len(hits) != 1 || hits[0].DocumentRef != "physics.txt" || hits[0].Ordinal != 2 || hits[0].Score != 0.91 {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}
