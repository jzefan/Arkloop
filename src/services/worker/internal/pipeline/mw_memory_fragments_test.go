package pipeline

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/memory"

	"github.com/google/uuid"
)

type fragmentSnapshotProviderStub struct {
	fragments []memory.MemoryFragment
}

func (p fragmentSnapshotProviderStub) Find(_ context.Context, _ memory.MemoryIdentity, _ string, _ string, _ int) ([]memory.MemoryHit, error) {
	return nil, nil
}

func (p fragmentSnapshotProviderStub) Content(_ context.Context, _ memory.MemoryIdentity, _ string, _ memory.MemoryLayer) (string, error) {
	return "", nil
}

func (p fragmentSnapshotProviderStub) ListDir(_ context.Context, _ memory.MemoryIdentity, _ string) ([]string, error) {
	return nil, nil
}

func (p fragmentSnapshotProviderStub) AppendSessionMessages(_ context.Context, _ memory.MemoryIdentity, _ string, _ []memory.MemoryMessage) error {
	return nil
}

func (p fragmentSnapshotProviderStub) CommitSession(_ context.Context, _ memory.MemoryIdentity, _ string) error {
	return nil
}

func (p fragmentSnapshotProviderStub) Write(_ context.Context, _ memory.MemoryIdentity, _ memory.MemoryScope, _ memory.MemoryEntry) error {
	return nil
}

func (p fragmentSnapshotProviderStub) Delete(_ context.Context, _ memory.MemoryIdentity, _ string) error {
	return nil
}

func (p fragmentSnapshotProviderStub) ListFragments(_ context.Context, _ memory.MemoryIdentity, _ int) ([]memory.MemoryFragment, error) {
	return append([]memory.MemoryFragment(nil), p.fragments...), nil
}

type snapshotStoreCapture struct {
	block string
	hits  []data.MemoryHitCache
}

func (s *snapshotStoreCapture) Get(context.Context, uuid.UUID, uuid.UUID, string) (string, bool, error) {
	return "", false, nil
}

func (s *snapshotStoreCapture) UpsertWithHits(_ context.Context, _, _ uuid.UUID, _ string, block string, hits []data.MemoryHitCache) error {
	s.block = block
	s.hits = append([]data.MemoryHitCache(nil), hits...)
	return nil
}

func TestRebuildSnapshotBlockUsesFragmentsWhenProviderSupportsIt(t *testing.T) {
	ident := memory.MemoryIdentity{
		AccountID: uuid.New(),
		UserID:    uuid.New(),
		AgentID:   "agent",
	}
	provider := fragmentSnapshotProviderStub{
		fragments: []memory.MemoryFragment{
			{
				ID:       "mem-1",
				URI:      "nowledge://memory/mem-1",
				Title:    "偏好",
				Content:  "用户偏好中文回复。",
				Abstract: "偏好",
				Score:    0.91,
			},
		},
	}

	block, hits, ok := rebuildSnapshotBlock(context.Background(), provider, ident, map[string][]string{
		string(memory.MemoryScopeUser): {"中文回复"},
	})
	if !ok {
		t.Fatal("expected fragment snapshot rebuild to succeed")
	}
	if block == "" {
		t.Fatal("expected non-empty memory block")
	}
	if len(hits) != 1 || hits[0].URI != "nowledge://memory/mem-1" || !hits[0].IsLeaf {
		t.Fatalf("unexpected hits: %#v", hits)
	}
}

func TestTryRefreshSnapshotFromQueriesWritesEmptyBlockForEmptyFragments(t *testing.T) {
	ident := memory.MemoryIdentity{
		AccountID: uuid.New(),
		UserID:    uuid.New(),
		AgentID:   "agent",
	}
	store := &snapshotStoreCapture{}
	ok, err := tryRefreshSnapshotFromQueries(context.Background(), store, fragmentSnapshotProviderStub{}, ident, map[string][]string{
		string(memory.MemoryScopeUser): {"empty"},
	})
	if err != nil {
		t.Fatalf("tryRefreshSnapshotFromQueries: %v", err)
	}
	if !ok {
		t.Fatal("expected refresh to report success for empty fragment snapshot")
	}
	if store.block != "" {
		t.Fatalf("expected empty block, got %q", store.block)
	}
	if len(store.hits) != 0 {
		t.Fatalf("expected no hits, got %#v", store.hits)
	}
}
