package catalogapi

import (
	"context"

	"arkloop/services/shared/objectstore"
)

type skillStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Head(ctx context.Context, key string) (objectstore.ObjectInfo, error)
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}
