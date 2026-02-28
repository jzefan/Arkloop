package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const artifactBucket = "sandbox-artifacts"

// ArtifactStore 管理 sandbox 执行产物的对象存储。
type ArtifactStore interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) error
	Get(ctx context.Context, key string) ([]byte, string, error) // data, contentType, error
}

// MinIOArtifactStore 基于 MinIO 实现 ArtifactStore。
type MinIOArtifactStore struct {
	client *minio.Client
}

// NewMinIOArtifactStore 创建 MinIOArtifactStore，复用已有的 MinIO 客户端创建逻辑。
func NewMinIOArtifactStore(ctx context.Context, endpoint, accessKey, secretKey string) (*MinIOArtifactStore, error) {
	secure, host := parseEndpoint(endpoint)

	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	store := &MinIOArtifactStore{client: client}
	if err := store.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *MinIOArtifactStore) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, artifactBucket)
	if err != nil {
		return fmt.Errorf("check bucket %q: %w", artifactBucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, artifactBucket, minio.MakeBucketOptions{}); err != nil {
		exists2, _ := s.client.BucketExists(ctx, artifactBucket)
		if !exists2 {
			return fmt.Errorf("make bucket %q: %w", artifactBucket, err)
		}
	}
	return nil
}

func (s *MinIOArtifactStore) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, artifactBucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload artifact %q: %w", key, err)
	}
	return nil
}

func (s *MinIOArtifactStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	obj, err := s.client.GetObject(ctx, artifactBucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("get artifact %q: %w", key, err)
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("stat artifact %q: %w", key, err)
	}

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", fmt.Errorf("read artifact %q: %w", key, err)
	}

	return data, info.ContentType, nil
}
