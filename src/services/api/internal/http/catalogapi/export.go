package catalogapi

import (
	"context"

	"arkloop/services/api/internal/data"
	"arkloop/services/shared/database"

	"github.com/google/uuid"
)

type ToolCatalogItem = toolCatalogItem

type ToolCatalogGroup = toolCatalogGroup

type ToolCatalogResponse = toolCatalogResponse

type PersonaResponse = personaResponse

type LLMProviderAvailableModelsResponse = llmProviderAvailableModelsResponse

func BuildEffectiveToolCatalogCompat(
	ctx context.Context,
	orgID uuid.UUID,
	overridesRepo *data.ToolDescriptionOverridesRepository,
	db database.DB,
	mcpCache *EffectiveToolCatalogCache,
	artifactStoreAvailable bool,
) (ToolCatalogResponse, error) {
	return buildEffectiveToolCatalog(ctx, orgID, overridesRepo, db, mcpCache, artifactStoreAvailable)
}
