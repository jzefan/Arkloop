package agentdirectory

import (
	"context"
	"encoding/json"
	"fmt"
)

type manifestEntry struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	SHA256  string `json:"sha256"`
	Deleted bool   `json:"deleted"`
}

type manifest struct {
	Entries []manifestEntry `json:"entries"`
}

// BlobStoreReader 是 objectstore.BlobStore 的读取子集。
type BlobStoreReader interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// ObjectStoreProvider 从 object store 读取 profile scope AWD 文件。
type ObjectStoreProvider struct {
	store       BlobStoreReader
	getRevision func(ctx context.Context, profileRef string) (string, error)
	workDirPath string
}

func NewObjectStoreProvider(
	store BlobStoreReader,
	getRevision func(ctx context.Context, profileRef string) (string, error),
	workDirPath string,
) *ObjectStoreProvider {
	return &ObjectStoreProvider{store: store, getRevision: getRevision, workDirPath: workDirPath}
}

func (p *ObjectStoreProvider) Load(ctx context.Context, profileRef string) (*Content, error) {
	if p.store == nil {
		return nil, nil
	}

	rev, err := p.getRevision(ctx, profileRef)
	if err != nil {
		return nil, fmt.Errorf("agentdirectory: get revision: %w", err)
	}
	if rev == "" {
		return &Content{WorkDirPath: p.workDirPath}, nil
	}

	manifestKey := "profiles/" + profileRef + "/manifests/" + rev + ".json"
	manifestData, err := p.store.Get(ctx, manifestKey)
	if err != nil {
		return nil, fmt.Errorf("agentdirectory: get manifest: %w", err)
	}

	var m manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return nil, fmt.Errorf("agentdirectory: unmarshal manifest: %w", err)
	}

	content := &Content{WorkDirPath: p.workDirPath}
	fieldMap := map[string]*string{
		"SOUL.md":   &content.Soul,
		"AGENTS.md": &content.Instructions,
		"MEMORY.md": &content.Memory,
		"USER.md":   &content.User,
	}

	for _, entry := range m.Entries {
		if entry.Deleted || entry.Type != "file" {
			continue
		}
		ptr, ok := fieldMap[entry.Path]
		if !ok {
			continue
		}
		blobKey := "profiles/" + profileRef + "/blobs/" + entry.SHA256
		data, err := p.store.Get(ctx, blobKey)
		if err != nil {
			continue
		}
		*ptr = string(data)
	}

	return content, nil
}
