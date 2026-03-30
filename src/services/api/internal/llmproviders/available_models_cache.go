package llmproviders

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultAvailableModelsCacheTTL = 2 * time.Minute

type availableModelsCacheKey struct {
	accountID  string
	providerID string
	scope      string
	userID     string
}

type availableModelsCacheEntry struct {
	models   []AvailableModel
	cachedAt time.Time
}

type availableModelsCache struct {
	ttl     time.Duration
	entries sync.Map
}

func newAvailableModelsCache(ttl time.Duration) *availableModelsCache {
	return &availableModelsCache{ttl: ttl}
}

func makeAvailableModelsCacheKey(accountID, providerID uuid.UUID, scope string, userID *uuid.UUID) availableModelsCacheKey {
	key := availableModelsCacheKey{
		accountID:  accountID.String(),
		providerID: providerID.String(),
		scope:      scope,
	}
	if userID != nil {
		key.userID = userID.String()
	}
	return key
}

func (c *availableModelsCache) getOrLoad(
	ctx context.Context,
	key availableModelsCacheKey,
	loader func(context.Context) ([]AvailableModel, error),
) ([]AvailableModel, error) {
	if c == nil || c.ttl <= 0 {
		return loader(ctx)
	}
	if cached, ok := c.get(key); ok {
		return cached, nil
	}
	models, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	c.entries.Store(key, availableModelsCacheEntry{
		models:   cloneAvailableModels(models),
		cachedAt: time.Now(),
	})
	return cloneAvailableModels(models), nil
}

func (c *availableModelsCache) get(key availableModelsCacheKey) ([]AvailableModel, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}
	value, ok := c.entries.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := value.(availableModelsCacheEntry)
	if !ok {
		c.entries.Delete(key)
		return nil, false
	}
	if time.Since(entry.cachedAt) >= c.ttl {
		c.entries.Delete(key)
		return nil, false
	}
	return cloneAvailableModels(entry.models), true
}

func (c *availableModelsCache) invalidateProvider(providerID uuid.UUID) {
	if c == nil {
		return
	}
	providerKey := providerID.String()
	c.entries.Range(func(key, _ any) bool {
		cacheKey, ok := key.(availableModelsCacheKey)
		if ok && cacheKey.providerID == providerKey {
			c.entries.Delete(key)
		}
		return true
	})
}

func cloneAvailableModels(models []AvailableModel) []AvailableModel {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]AvailableModel, len(models))
	for idx, model := range models {
		cloned[idx] = model
		cloned[idx].ContextLength = cloneOptionalInt(model.ContextLength)
		cloned[idx].MaxOutputTokens = cloneOptionalInt(model.MaxOutputTokens)
		if model.InputModalities != nil {
			cloned[idx].InputModalities = append([]string(nil), model.InputModalities...)
		}
		if model.OutputModalities != nil {
			cloned[idx].OutputModalities = append([]string(nil), model.OutputModalities...)
		}
	}
	return cloned
}

func cloneOptionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
