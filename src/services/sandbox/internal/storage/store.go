package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"arkloop/services/shared/objectstore"
	"golang.org/x/sync/singleflight"
)

const snapshotBucket = "sandbox-snapshots"

type SnapshotStore interface {
	Upload(ctx context.Context, templateID string, memPath, diskPath string) error
	Download(ctx context.Context, templateID string) (memPath, diskPath string, err error)
	Exists(ctx context.Context, templateID string) (bool, error)
}

type etagCache struct {
	MemETag  string `json:"mem_etag"`
	DiskETag string `json:"disk_etag"`
}

type ObjectSnapshotStore struct {
	store        objectstore.Store
	cacheBaseDir string
	dlGroup      singleflight.Group
}

func NewSnapshotStore(ctx context.Context, opener objectstore.BucketOpener, cacheBaseDir string) (*ObjectSnapshotStore, error) {
	if opener == nil {
		return nil, fmt.Errorf("bucket opener must not be nil")
	}
	cacheBaseDir = strings.TrimSpace(cacheBaseDir)
	if cacheBaseDir == "" {
		return nil, fmt.Errorf("cache base dir must not be empty")
	}
	store, err := opener.Open(ctx, snapshotBucket)
	if err != nil {
		return nil, err
	}
	return &ObjectSnapshotStore{store: store, cacheBaseDir: cacheBaseDir}, nil
}

func (s *ObjectSnapshotStore) Upload(ctx context.Context, templateID string, memPath, diskPath string) error {
	if err := s.uploadFile(ctx, templateID, "mem.snap", memPath); err != nil {
		return err
	}
	return s.uploadFile(ctx, templateID, "disk.snap", diskPath)
}

func (s *ObjectSnapshotStore) uploadFile(ctx context.Context, templateID, fileName, localPath string) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", localPath, err)
	}
	key := templateID + "/" + fileName
	if err := s.store.PutObject(ctx, key, data, objectstore.PutOptions{ContentType: "application/octet-stream"}); err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

func (s *ObjectSnapshotStore) Download(ctx context.Context, templateID string) (memPath, diskPath string, err error) {
	type result struct {
		memPath  string
		diskPath string
	}
	v, err, _ := s.dlGroup.Do(templateID, func() (any, error) {
		mem, disk, innerErr := s.download(ctx, templateID)
		if innerErr != nil {
			return nil, innerErr
		}
		return &result{memPath: mem, diskPath: disk}, nil
	})
	if err != nil {
		return "", "", err
	}
	r := v.(*result)
	return r.memPath, r.diskPath, nil
}

func (s *ObjectSnapshotStore) download(ctx context.Context, templateID string) (memPath, diskPath string, err error) {
	cacheDir := filepath.Join(s.cacheBaseDir, templateID)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create cache dir: %w", err)
	}

	memLocal := filepath.Join(cacheDir, "mem.snap")
	diskLocal := filepath.Join(cacheDir, "disk.snap")
	cachePath := filepath.Join(cacheDir, "etag.json")

	memKey := templateID + "/mem.snap"
	diskKey := templateID + "/disk.snap"

	memInfo, err := s.store.Head(ctx, memKey)
	if err != nil {
		return "", "", fmt.Errorf("stat mem.snap: %w", err)
	}
	diskInfo, err := s.store.Head(ctx, diskKey)
	if err != nil {
		return "", "", fmt.Errorf("stat disk.snap: %w", err)
	}

	cached := s.loadETagCache(cachePath)
	if cached.MemETag != memInfo.ETag || !fileExists(memLocal) {
		data, err := s.store.Get(ctx, memKey)
		if err != nil {
			return "", "", fmt.Errorf("download mem.snap: %w", err)
		}
		if err := os.WriteFile(memLocal, data, 0o600); err != nil {
			return "", "", fmt.Errorf("write mem.snap: %w", err)
		}
	}
	if cached.DiskETag != diskInfo.ETag || !fileExists(diskLocal) {
		data, err := s.store.Get(ctx, diskKey)
		if err != nil {
			return "", "", fmt.Errorf("download disk.snap: %w", err)
		}
		if err := os.WriteFile(diskLocal, data, 0o600); err != nil {
			return "", "", fmt.Errorf("write disk.snap: %w", err)
		}
	}

	_ = s.saveETagCache(cachePath, etagCache{MemETag: memInfo.ETag, DiskETag: diskInfo.ETag})
	return memLocal, diskLocal, nil
}

func (s *ObjectSnapshotStore) Exists(ctx context.Context, templateID string) (bool, error) {
	for _, suffix := range []string{"/mem.snap", "/disk.snap"} {
		key := templateID + suffix
		if _, err := s.store.Head(ctx, key); err != nil {
			if objectstore.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("stat %s: %w", key, err)
		}
	}
	return true, nil
}

func (s *ObjectSnapshotStore) loadETagCache(path string) etagCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return etagCache{}
	}
	var cached etagCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return etagCache{}
	}
	return cached
}

func (s *ObjectSnapshotStore) saveETagCache(path string, cached etagCache) error {
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var _ SnapshotStore = (*ObjectSnapshotStore)(nil)
