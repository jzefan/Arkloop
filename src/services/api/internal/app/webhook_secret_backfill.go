package app

import (
	"context"
	"fmt"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/observability"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func backfillWebhookSecrets(
	ctx context.Context,
	pool *pgxpool.Pool,
	webhookRepo *data.WebhookEndpointRepository,
	secretsRepo *data.SecretsRepository,
	logger *observability.JSONLogger,
) error {
	if ctx == nil || pool == nil || webhookRepo == nil || secretsRepo == nil {
		return nil
	}

	legacyEndpoints, err := webhookRepo.ListLegacySecrets(ctx)
	if err != nil {
		return fmt.Errorf("list legacy webhooks: %w", err)
	}
	if len(legacyEndpoints) == 0 {
		return nil
	}

	migrated := 0
	for _, endpoint := range legacyEndpoints {
		if endpoint.ID == uuid.Nil || endpoint.OrgID == uuid.Nil || endpoint.SigningSecret == nil || *endpoint.SigningSecret == "" {
			continue
		}

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin webhook secret backfill tx: %w", err)
		}

		txSecrets := secretsRepo.WithTx(tx)
		txWebhooks := webhookRepo.WithTx(tx)
		secret, err := txSecrets.Upsert(ctx, endpoint.OrgID, data.WebhookSecretName(endpoint.ID), *endpoint.SigningSecret)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("upsert webhook secret %s: %w", endpoint.ID, err)
		}
		if err := txWebhooks.AttachSecret(ctx, endpoint.ID, secret.ID); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("attach webhook secret %s: %w", endpoint.ID, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit webhook secret backfill %s: %w", endpoint.ID, err)
		}
		migrated++
	}

	if logger != nil && migrated > 0 {
		logger.Info("webhook secret backfill", observability.LogFields{}, map[string]any{"count": migrated})
	}
	return nil
}
