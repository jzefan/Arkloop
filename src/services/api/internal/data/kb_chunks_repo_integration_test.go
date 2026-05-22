//go:build !desktop

package data

import (
	"context"
	"math"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
)

func setupChunksRepo(t *testing.T) (*KBChunksRepository, *KBDocumentsRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
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
	docs, _ := NewKBDocumentsRepository(pool)
	kbs, _ := NewKnowledgeBasesRepository(pool)
	accts, _ := NewAccountRepository(pool)
	return repo, docs, kbs, accts, ctx
}

// makeVec returns a normalized fake vector of dim D where slot pos is 1.0
// and all other slots are 0. Cosine similarity is 1.0 when pos matches, 0.0 otherwise.
func makeVec(dim, pos int) []float32 {
	v := make([]float32, dim)
	v[pos] = 1.0
	return v
}

func TestKBChunksUpsertAndSearch(t *testing.T) {
	chunks, docs, kbs, accts, ctx := setupChunksRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb"})
	doc, _ := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "physics.txt", MimeType: "text/plain", BlobSHA256: "sha", SizeBytes: 1})

	dim := chunks.Dim()
	if dim <= 0 {
		t.Fatalf("chunks.Dim()=%d, expected >0", dim)
	}
	in := []KBChunkUpsert{
		{KBID: kb.ID, DocumentID: doc.ID, Ordinal: 0, ChunkType: "paragraph", Text: "光的干涉...", TokenCount: 42, Embedding: makeVec(dim, 0)},
		{KBID: kb.ID, DocumentID: doc.ID, Ordinal: 1, ChunkType: "paragraph", Text: "电磁感应...", TokenCount: 51, Embedding: makeVec(dim, 1)},
	}
	if err := chunks.Upsert(ctx, in); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	hits, err := chunks.Search(ctx, kb.ID, makeVec(dim, 1), 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].DocumentRef != "physics.txt" || hits[0].Ordinal != 1 {
		t.Errorf("top hit: got (%s,%d), want (physics.txt,1)", hits[0].DocumentRef, hits[0].Ordinal)
	}
	if math.Abs(float64(hits[0].Score-1.0)) > 0.01 {
		t.Errorf("top score should be ~1.0 (cosine), got %f", hits[0].Score)
	}
}

func TestKBChunksUpsertIsIdempotent(t *testing.T) {
	chunks, docs, kbs, accts, ctx := setupChunksRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb"})
	doc, _ := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "d.txt", MimeType: "text/plain", BlobSHA256: "sha", SizeBytes: 1})
	dim := chunks.Dim()
	row := KBChunkUpsert{KBID: kb.ID, DocumentID: doc.ID, Ordinal: 0, ChunkType: "paragraph", Text: "first", TokenCount: 10, Embedding: makeVec(dim, 0)}
	if err := chunks.Upsert(ctx, []KBChunkUpsert{row}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	row.Text = "updated"
	if err := chunks.Upsert(ctx, []KBChunkUpsert{row}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	hits, err := chunks.Search(ctx, kb.ID, makeVec(dim, 0), 5)
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

func TestKBChunksIsolatedByKB(t *testing.T) {
	chunks, docs, kbs, accts, ctx := setupChunksRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kbA, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb-a"})
	kbB, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb-b"})
	docA, _ := docs.Create(ctx, DocCreate{KBID: kbA.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "a", SizeBytes: 1})
	docB, _ := docs.Create(ctx, DocCreate{KBID: kbB.ID, OriginalFilename: "b.txt", MimeType: "text/plain", BlobSHA256: "b", SizeBytes: 1})

	dim := chunks.Dim()
	if err := chunks.Upsert(ctx, []KBChunkUpsert{
		{KBID: kbA.ID, DocumentID: docA.ID, Ordinal: 0, ChunkType: "paragraph", Text: "A", TokenCount: 1, Embedding: makeVec(dim, 0)},
		{KBID: kbB.ID, DocumentID: docB.ID, Ordinal: 0, ChunkType: "paragraph", Text: "B", TokenCount: 1, Embedding: makeVec(dim, 0)},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	hits, err := chunks.Search(ctx, kbA.ID, makeVec(dim, 0), 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Text != "A" {
		t.Fatalf("kb isolation broken: %+v", hits)
	}
}
