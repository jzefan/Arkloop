package pipeline

import (
	"context"
	"log/slog"
	"strings"

	sharedtoolmeta "arkloop/services/shared/toolmeta"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"

	"github.com/google/uuid"
)

type ToolDescriptionOverridesReader interface {
	ListByScope(ctx context.Context, orgID uuid.UUID, scope string) ([]data.ToolDescriptionOverride, error)
}

func NewToolDescriptionOverrideMiddleware(repo ToolDescriptionOverridesReader) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if repo == nil {
			return next(ctx, rc)
		}

		platformOverrides, err := repo.ListByScope(ctx, uuid.Nil, "platform")
		if err != nil {
			slog.WarnContext(ctx, "tool description override load failed", "run_id", rc.Run.ID, "scope", "platform", "err", err.Error())
			return next(ctx, rc)
		}

		orgOverrides := []data.ToolDescriptionOverride{}
		if rc.Run.OrgID != uuid.Nil {
			orgOverrides, err = repo.ListByScope(ctx, rc.Run.OrgID, "org")
			if err != nil {
				slog.WarnContext(ctx, "tool description override load failed", "run_id", rc.Run.ID, "scope", "org", "org_id", rc.Run.OrgID, "err", err.Error())
				return next(ctx, rc)
			}
		}

		descriptionByTool := make(map[string]string, len(platformOverrides)+len(orgOverrides))
		for _, override := range platformOverrides {
			if strings.TrimSpace(override.Description) == "" {
				continue
			}
			descriptionByTool[override.ToolName] = override.Description
		}
		for _, override := range orgOverrides {
			if strings.TrimSpace(override.Description) == "" {
				continue
			}
			descriptionByTool[override.ToolName] = override.Description
		}
		if len(descriptionByTool) == 0 {
			return next(ctx, rc)
		}

		rc.ToolSpecs = applyToolDescriptionOverrides(rc.ToolSpecs, descriptionByTool)
		return next(ctx, rc)
	}
}

func applyToolDescriptionOverrides(specs []llm.ToolSpec, descriptionByTool map[string]string) []llm.ToolSpec {
	if len(specs) == 0 || len(descriptionByTool) == 0 {
		return specs
	}

	out := append([]llm.ToolSpec(nil), specs...)
	for i := range out {
		if _, ok := sharedtoolmeta.Lookup(out[i].Name); !ok {
			continue
		}
		description, ok := descriptionByTool[out[i].Name]
		if !ok {
			continue
		}
		out[i].Description = StringPtr(description)
	}
	return out
}
