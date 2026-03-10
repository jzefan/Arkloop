package catalogapi

import "time"

type EffectiveToolCatalogCache = effectiveToolCatalogCache

const EffectiveToolCatalogTTL = effectiveToolCatalogTTL

func NewEffectiveToolCatalogCache(ttl time.Duration) *EffectiveToolCatalogCache {
	return newEffectiveToolCatalogCache(ttl)
}
