package kb

import (
	"context"
	"testing"

	wdata "arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type fakeChunksRepo struct{ hits []wdata.KBChunkHit }

func (f *fakeChunksRepo) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]wdata.KBChunkHit, error) {
	return f.hits, nil
}

func (f *fakeChunksRepo) Dim() int { return 8 }

type fakeEmbedder struct{ dim int }

func (e fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, e.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}

func (e fakeEmbedder) Dim() int { return e.dim }

type fakeAccess struct{ allow bool }

func (f fakeAccess) IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error) {
	return f.allow, nil
}

func TestKBSearchHappyPath(t *testing.T) {
	kbID := uuid.New()
	exe := NewExecutor(&fakeChunksRepo{hits: []wdata.KBChunkHit{
		{DocumentRef: "physics.txt", Ordinal: 3, Text: "光的干涉", Score: 0.91, Metadata: map[string]any{"page": 2}},
		{DocumentRef: "physics.txt", Ordinal: 4, Text: "杨氏双缝", Score: 0.74},
	}}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	accountID := uuid.New()

	result := exe.Execute(context.Background(), ToolNameSearch, map[string]any{"kb_id": kbID.String(), "query": "干涉", "k": 5}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error != nil {
		t.Fatalf("execute error: %v", result.Error)
	}
	hits, _ := result.ResultJSON["hits"].([]map[string]any)
	if len(hits) != 2 {
		t.Errorf("got %d hits", len(hits))
	}
	meta, _ := hits[0]["metadata"].(map[string]any)
	if meta["page"] != 2 {
		t.Fatalf("metadata not returned: %+v", hits[0])
	}
}

func TestKBSearchDeniesNonMember(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: false})
	accountID := uuid.New()
	result := exe.Execute(context.Background(), ToolNameSearch, map[string]any{"kb_id": uuid.New().String(), "query": "x"}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error == nil || result.Error.ErrorClass != errorPermissionDenied {
		t.Fatalf("expected permission denied, got %+v", result.Error)
	}
}

func TestKBSearchValidatesInput(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	accountID := uuid.New()
	result := exe.Execute(context.Background(), ToolNameSearch, map[string]any{"query": "x"}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error == nil || result.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("expected args error for missing kb_id, got %+v", result.Error)
	}
	result = exe.Execute(context.Background(), ToolNameSearch, map[string]any{"kb_id": uuid.New().String()}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error == nil || result.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("expected args error for missing query, got %+v", result.Error)
	}
}
