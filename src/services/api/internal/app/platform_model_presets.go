//go:build !desktop

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type platformModelPresetSpec struct {
	Provider   string
	Name       string
	APIKey     string
	SecretName string
	Models     []string
	BaseURL    *string
	Priority   int
}

func syncPlatformModelPresets(
	ctx context.Context,
	db data.Querier,
	credentialsRepo *data.LlmCredentialsRepository,
	routesRepo *data.LlmRoutesRepository,
	secretsRepo *data.SecretsRepository,
	logger *slog.Logger,
) error {
	if db == nil || credentialsRepo == nil || routesRepo == nil || secretsRepo == nil {
		return nil
	}
	specs := platformModelPresetSpecsFromEnv(os.Getenv)
	if len(specs) == 0 {
		return nil
	}
	for _, spec := range specs {
		if err := syncPlatformModelPreset(ctx, db, credentialsRepo, routesRepo, secretsRepo, spec); err != nil {
			return err
		}
		if logger != nil {
			logger.InfoContext(ctx, "platform model preset synced",
				"provider", spec.Provider,
				"credential_name", spec.Name,
				"models", strings.Join(spec.Models, ","),
			)
		}
	}
	return nil
}

func platformModelPresetSpecsFromEnv(getenv func(string) string) []platformModelPresetSpec {
	if getenv == nil {
		getenv = os.Getenv
	}
	defs := []struct {
		provider   string
		name       string
		keyEnvs    []string
		modelsEnv  string
		baseURLEnv string
		secretName string
		models     []string
		priority   int
	}{
		{
			provider:   "deepseek",
			name:       "DeepSeek",
			keyEnvs:    []string{"ARKLOOP_DEEPSEEK_API_KEY", "DEEPSEEK_API_KEY"},
			modelsEnv:  "ARKLOOP_DEEPSEEK_MODELS",
			baseURLEnv: "ARKLOOP_DEEPSEEK_BASE_URL",
			secretName: "llm:deepseek",
			models:     []string{"deepseek-v4-flash", "deepseek-v4-pro"},
			priority:   300,
		},
		{
			provider:   "qwen",
			name:       "Qwen",
			keyEnvs:    []string{"ARKLOOP_QWEN_API_KEY", "QWEN_API_KEY", "DASHSCOPE_API_KEY"},
			modelsEnv:  "ARKLOOP_QWEN_MODELS",
			baseURLEnv: "ARKLOOP_QWEN_BASE_URL",
			secretName: "llm:qwen",
			// Stable DashScope OpenAI-compatible model IDs (Qwen3 generation).
			models:   []string{"qwen3.5-plus", "qwen3-max-2026-01-23"},
			priority: 200,
		},
		{
			provider:   "doubao",
			name:       "Doubao",
			keyEnvs:    []string{"ARKLOOP_DOUBAO_API_KEY", "DOUBAO_API_KEY", "ARK_API_KEY", "VOLCENGINE_API_KEY"},
			modelsEnv:  "ARKLOOP_DOUBAO_MODELS",
			baseURLEnv: "ARKLOOP_DOUBAO_BASE_URL",
			secretName: "llm:doubao",
			// Volcengine Ark requires either a dated foundation model ID
			// (e.g. doubao-seed-2-0-lite-260428) or a user-created endpoint
			// ID (ep-xxxxxxxx). The dated default below matches the ID
			// surfaced by the Ark console for an opened-up Seed-2.0-lite
			// subscription; operators on different plans must override via
			// ARKLOOP_DOUBAO_MODELS for their own Ark account.
			models:   []string{"doubao-seed-2-0-lite-260428", "doubao-seed-2-0-mini-260428"},
			priority: 100,
		},
	}

	out := make([]platformModelPresetSpec, 0, len(defs))
	for _, def := range defs {
		apiKey := firstNonEmptyEnv(getenv, def.keyEnvs...)
		if apiKey == "" {
			continue
		}
		models := splitModelList(getenv(def.modelsEnv), def.models)
		if len(models) == 0 {
			continue
		}
		var baseURL *string
		if raw := strings.TrimRight(strings.TrimSpace(getenv(def.baseURLEnv)), "/"); raw != "" {
			baseURL = &raw
		}
		out = append(out, platformModelPresetSpec{
			Provider:   def.provider,
			Name:       def.name,
			APIKey:     apiKey,
			SecretName: def.secretName,
			Models:     models,
			BaseURL:    baseURL,
			Priority:   def.priority,
		})
	}
	return out
}

func firstNonEmptyEnv(getenv func(string) string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func splitModelList(raw string, fallback []string) []string {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), fallback...)
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		model := strings.TrimSpace(part)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func syncPlatformModelPreset(
	ctx context.Context,
	db data.Querier,
	credentialsRepo *data.LlmCredentialsRepository,
	routesRepo *data.LlmRoutesRepository,
	secretsRepo *data.SecretsRepository,
	spec platformModelPresetSpec,
) error {
	secret, err := secretsRepo.UpsertPlatform(ctx, spec.SecretName, spec.APIKey)
	if err != nil {
		return fmt.Errorf("sync platform model preset secret %s: %w", spec.Provider, err)
	}

	credential, err := findPlatformCredentialByName(ctx, credentialsRepo, spec.Name)
	if err != nil {
		return err
	}
	if credential == nil {
		created, err := credentialsRepo.Create(ctx, uuid.New(), "platform", nil, spec.Provider, spec.Name, &secret.ID, nil, spec.BaseURL, nil, map[string]any{})
		if err != nil {
			return fmt.Errorf("create platform credential %s: %w", spec.Provider, err)
		}
		credential = &created
	} else {
		updated, err := credentialsRepo.Update(ctx, "platform", nil, credential.ID, spec.Provider, spec.Name, spec.BaseURL, nil, credential.AdvancedJSON)
		if err != nil {
			return fmt.Errorf("update platform credential %s: %w", spec.Provider, err)
		}
		if updated != nil {
			credential = updated
		}
	}
	if err := credentialsRepo.UpdateSecret(ctx, "platform", nil, credential.ID, &secret.ID, nil); err != nil {
		return fmt.Errorf("update platform credential secret %s: %w", spec.Provider, err)
	}

	for idx, model := range spec.Models {
		isDefault := idx == 0
		priority := spec.Priority - idx
		if err := upsertPlatformRoute(ctx, db, routesRepo, credential.ID, model, priority, isDefault); err != nil {
			return fmt.Errorf("upsert platform route %s/%s: %w", spec.Provider, model, err)
		}
	}
	return nil
}

func findPlatformCredentialByName(ctx context.Context, repo *data.LlmCredentialsRepository, name string) (*data.LlmCredential, error) {
	credentials, err := repo.ListByOwner(ctx, "platform", nil)
	if err != nil {
		return nil, fmt.Errorf("list platform credentials: %w", err)
	}
	for i := range credentials {
		if credentials[i].Name == name {
			return &credentials[i], nil
		}
	}
	return nil, nil
}

func upsertPlatformRoute(ctx context.Context, db data.Querier, repo *data.LlmRoutesRepository, credentialID uuid.UUID, model string, priority int, isDefault bool) error {
	routes, err := repo.ListByCredential(ctx, uuid.Nil, credentialID, data.LlmRouteScopePlatform)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if route.Model != model {
			continue
		}
		if isDefault {
			if _, execErr := db.Exec(ctx, `UPDATE llm_routes SET is_default = FALSE WHERE credential_id = $1 AND id <> $2`, credentialID, route.ID); execErr != nil {
				return execErr
			}
		}
		_, err := repo.Update(ctx, data.UpdateLlmRouteParams{
			Scope:               data.LlmRouteScopePlatform,
			RouteID:             route.ID,
			Model:               model,
			Priority:            priority,
			IsDefault:           isDefault,
			ShowInPicker:        true,
			Tags:                []string{"platform", "preset"},
			WhenJSON:            json.RawMessage("{}"),
			AdvancedJSON:        route.AdvancedJSON,
			Multiplier:          route.Multiplier,
			CostPer1kInput:      route.CostPer1kInput,
			CostPer1kOutput:     route.CostPer1kOutput,
			CostPer1kCacheWrite: route.CostPer1kCacheWrite,
			CostPer1kCacheRead:  route.CostPer1kCacheRead,
		})
		return err
	}

	_, err = repo.Create(ctx, data.CreateLlmRouteParams{
		Scope:        data.LlmRouteScopePlatform,
		CredentialID: credentialID,
		Model:        model,
		Priority:     priority,
		IsDefault:    isDefault,
		ShowInPicker: true,
		Tags:         []string{"platform", "preset"},
		WhenJSON:     json.RawMessage("{}"),
		AdvancedJSON: map[string]any{},
		Multiplier:   1.0,
	})
	if err == nil || !isLlmRouteDefaultConflict(err) {
		return err
	}
	if _, execErr := db.Exec(ctx, `UPDATE llm_routes SET is_default = FALSE WHERE credential_id = $1`, credentialID); execErr != nil {
		return execErr
	}
	_, err = repo.Create(ctx, data.CreateLlmRouteParams{
		Scope:        data.LlmRouteScopePlatform,
		CredentialID: credentialID,
		Model:        model,
		Priority:     priority,
		IsDefault:    isDefault,
		ShowInPicker: true,
		Tags:         []string{"platform", "preset"},
		WhenJSON:     json.RawMessage("{}"),
		AdvancedJSON: map[string]any{},
		Multiplier:   1.0,
	})
	return err
}

func isLlmRouteDefaultConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ux_llm_routes_credential_default")
}
