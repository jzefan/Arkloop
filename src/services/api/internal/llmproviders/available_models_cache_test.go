package llmproviders

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAvailableModelsCacheGetOrLoadCachesByKey(t *testing.T) {
	cache := newAvailableModelsCache(time.Minute)
	key := makeAvailableModelsCacheKey(uuid.New(), uuid.New(), "user", nil)
	calls := 0

	loader := func(context.Context) ([]AvailableModel, error) {
		calls++
		return []AvailableModel{{ID: "openai/gpt-4o-mini", Name: "GPT-4o mini"}}, nil
	}

	first, err := cache.getOrLoad(context.Background(), key, loader)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := cache.getOrLoad(context.Background(), key, loader)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected loader once, got %d", calls)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("unexpected models: %#v %#v", first, second)
	}
}

func TestAvailableModelsCacheInvalidateProvider(t *testing.T) {
	cache := newAvailableModelsCache(time.Minute)
	providerID := uuid.New()
	key := makeAvailableModelsCacheKey(uuid.New(), providerID, "user", nil)
	otherKey := makeAvailableModelsCacheKey(uuid.New(), uuid.New(), "user", nil)

	cache.entries.Store(key, availableModelsCacheEntry{models: []AvailableModel{{ID: "a"}}, cachedAt: time.Now()})
	cache.entries.Store(otherKey, availableModelsCacheEntry{models: []AvailableModel{{ID: "b"}}, cachedAt: time.Now()})

	cache.invalidateProvider(providerID)

	if _, ok := cache.get(key); ok {
		t.Fatal("expected provider cache invalidated")
	}
	if _, ok := cache.get(otherKey); !ok {
		t.Fatal("expected other provider cache retained")
	}
}
