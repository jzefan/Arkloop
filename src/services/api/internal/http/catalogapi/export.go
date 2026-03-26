package catalogapi

import (
	"context"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type ToolCatalogItem = toolCatalogItem

type ToolCatalogGroup = toolCatalogGroup

type ToolCatalogResponse = toolCatalogResponse

type PersonaResponse = personaResponse

type LLMProviderAvailableModelsResponse = llmProviderAvailableModelsResponse

func BuildEffectiveToolCatalog(
	ctx context.Context,
	accountID uuid.UUID,
	userID uuid.UUID,
	projectID uuid.UUID,
	overridesRepo *data.ToolDescriptionOverridesRepository,
	pool data.DB,
	mcpCache *EffectiveToolCatalogCache,
	artifactStoreAvailable bool,
) (ToolCatalogResponse, error) {
	return buildEffectiveToolCatalog(ctx, accountID, userID, projectID, overridesRepo, pool, mcpCache, artifactStoreAvailable)
}
