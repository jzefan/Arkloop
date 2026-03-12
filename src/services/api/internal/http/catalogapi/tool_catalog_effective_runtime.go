package catalogapi

import (
	"context"
	"fmt"

	apicrypto "arkloop/services/api/internal/crypto"
	sharedconfig "arkloop/services/shared/config"
	sharedtoolruntime "arkloop/services/shared/toolruntime"
	"arkloop/services/shared/database"

	"log/slog"
)

func buildEffectiveBuiltinToolNameSet(
	ctx context.Context,
	db database.DB,
	artifactStoreAvailable bool,
) map[string]struct{} {
	resolver, _ := sharedconfig.NewResolver(
		sharedconfig.DefaultRegistry(),
		sharedconfig.NewPGXStore(db),
		nil,
		0,
	)

	snapshot, err := sharedtoolruntime.BuildRuntimeSnapshot(ctx, sharedtoolruntime.SnapshotInput{
		ConfigResolver:         resolver,
		HasConversationSearch:  db != nil,
		ArtifactStoreAvailable: artifactStoreAvailable,
		LoadPlatformProviders: func(loadCtx context.Context) ([]sharedtoolruntime.ProviderConfig, error) {
			return sharedtoolruntime.LoadPlatformProviders(loadCtx, db, decryptPlatformProviderSecret)
		},
	})
	if err != nil {
		slog.WarnContext(ctx, "effective tool catalog: runtime snapshot build failed", "err", err.Error())
		return map[string]struct{}{}
	}
	return snapshot.BuiltinToolNameSet()
}

func decryptPlatformProviderSecret(ctx context.Context, encrypted string, keyVersion *int, providerName string) (*string, error) {
	_ = ctx
	if keyVersion == nil {
		return nil, fmt.Errorf("tool_provider_configs decrypt: missing key version for %s", providerName)
	}
	keyRing, err := apicrypto.NewKeyRingFromEnv()
	if err != nil {
		return nil, fmt.Errorf("tool_provider_configs decrypt: %w", err)
	}
	plaintext, err := keyRing.Decrypt(encrypted, *keyVersion)
	if err != nil {
		return nil, fmt.Errorf("tool_provider_configs decrypt: %w", err)
	}
	value := string(plaintext)
	return &value, nil
}
