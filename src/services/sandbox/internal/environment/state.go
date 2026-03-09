package environment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"arkloop/services/sandbox/internal/environment/contract"
	"arkloop/services/shared/objectstore"
	"arkloop/services/shared/workspaceblob"
)

func manifestKey(scope, ref, revision string) string {
	switch strings.TrimSpace(scope) {
	case ScopeProfile:
		return contract.ProfileManifestKey(ref, revision)
	case ScopeWorkspace:
		return contract.WorkspaceManifestKey(ref, revision)
	default:
		return ""
	}
}

func blobKey(scope, ref, sha256 string) string {
	switch strings.TrimSpace(scope) {
	case ScopeProfile:
		return contract.ProfileBlobKey(ref, sha256)
	case ScopeWorkspace:
		return contract.WorkspaceBlobKey(ref, sha256)
	default:
		return ""
	}
}

func blobPrefix(scope, ref string) string {
	switch strings.TrimSpace(scope) {
	case ScopeProfile:
		return "profiles/" + strings.TrimSpace(ref) + "/blobs/"
	case ScopeWorkspace:
		return "workspaces/" + strings.TrimSpace(ref) + "/blobs/"
	default:
		return ""
	}
}

func loadManifest(ctx context.Context, store objectstore.BlobStore, scope, ref, revision string) (*Manifest, error) {
	data, err := store.Get(ctx, manifestKey(scope, ref, revision))
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	normalized := NormalizeManifest(manifest)
	if normalized.Scope != strings.TrimSpace(scope) || normalized.Ref != strings.TrimSpace(ref) {
		return nil, fmt.Errorf("manifest identity mismatch")
	}
	if normalized.Revision != strings.TrimSpace(revision) {
		return nil, fmt.Errorf("manifest revision mismatch")
	}
	return &normalized, nil
}

func saveManifest(ctx context.Context, store objectstore.BlobStore, manifest Manifest) error {
	normalized := NormalizeManifest(manifest)
	data, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := store.Put(ctx, manifestKey(normalized.Scope, normalized.Ref, normalized.Revision), data); err != nil {
		return fmt.Errorf("put manifest: %w", err)
	}
	return nil
}

func loadBlob(ctx context.Context, store objectstore.BlobStore, key string) ([]byte, error) {
	encoded, err := store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return workspaceblob.Decode(encoded)
}

func putBlobIfMissing(ctx context.Context, store objectstore.BlobStore, key string, data []byte) (bool, error) {
	if key == "" {
		return false, fmt.Errorf("blob key must not be empty")
	}
	encoded, err := workspaceblob.Encode(data)
	if err != nil {
		return false, err
	}
	created, err := store.PutIfAbsent(ctx, key, encoded)
	if err != nil {
		return false, err
	}
	return created, nil
}

func hydrateScope(ctx context.Context, store objectstore.BlobStore, carrier Carrier, scope, ref, revision string) error {
	manifest, files, err := loadHydratedScope(ctx, store, scope, ref, revision)
	if err != nil {
		return err
	}
	return carrier.ApplyEnvironment(ctx, scope, manifest, files, true)
}

func loadHydratedScope(ctx context.Context, store objectstore.BlobStore, scope, ref, revision string) (Manifest, []FilePayload, error) {
	manifest, err := loadManifest(ctx, store, scope, ref, revision)
	if err != nil {
		return Manifest{}, nil, err
	}
	hydrated := BuildHydrateManifest(scope, *manifest, PrepareOptions{WorkspaceMode: WorkspaceHydrationFull})
	files := make([]FilePayload, 0)
	for _, entry := range hydrated.Entries {
		if entry.Type != EntryTypeFile || strings.TrimSpace(entry.SHA256) == "" || entry.Deleted {
			continue
		}
		data, err := loadBlob(ctx, store, blobKey(scope, ref, entry.SHA256))
		if err != nil {
			return Manifest{}, nil, err
		}
		files = append(files, EncodeFilePayload(entry.Path, data, entry))
	}
	return hydrated, files, nil
}
