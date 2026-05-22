package kbapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type fakeChunks struct {
	hits []data.KBChunkHit
}

func (f *fakeChunks) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]data.KBChunkHit, error) {
	return f.hits, nil
}

type fakeEmbed struct{}

func (fakeEmbed) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1.0, 0.0}
	}
	return out, nil
}

func TestSearchHappyPath(t *testing.T) {
	ctx := &handlerCtx{
		kbStore:    newFakeKBStore(),
		membership: &fakeMembership{allow: true},
		chunksRepo: &fakeChunks{hits: []data.KBChunkHit{{DocumentRef: "a.txt", Ordinal: 0, Text: "光的干涉", Score: 0.92}}},
		embedder:   fakeEmbed{},
	}
	acc := uuid.New()
	kb, _ := ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "w", Name: "n"})
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/search?q=light&k=3", nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleSearch(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hits []map[string]any `json:"hits"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Hits) != 1 {
		t.Errorf("got %d hits", len(resp.Hits))
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	ctx := &handlerCtx{kbStore: newFakeKBStore(), membership: &fakeMembership{allow: true}}
	acc := uuid.New()
	kb, _ := ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "w", Name: "n"})
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/search", nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleSearch(ctx)(w, req)
	if w.Code != 400 {
		t.Errorf("got %d", w.Code)
	}
}
