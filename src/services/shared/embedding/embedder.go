// Package embedding provides text-to-vector embeddings. M0 implements the
// Doubao Ark backend; the interface is named so M1 can add OpenAI /
// OpenViking implementations without breaking callers.
package embedding

import (
	"context"
	"errors"
)

// Embedder turns text into fixed-dimension float32 vectors.
type Embedder interface {
	// Embed returns one vector per input in input order. nil input returns nil output.
	// Returned vectors are guaranteed to have length Dim().
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dim is the dimension of every vector this Embedder returns.
	Dim() int
}

// ErrUpstream indicates the upstream provider failed after the retry budget.
var ErrUpstream = errors.New("embedding: upstream failed after retries")

// ErrDimMismatch indicates the server returned a vector of a different size
// than the configured Dim. This protects pgvector(N) against silent drift.
var ErrDimMismatch = errors.New("embedding: provider returned unexpected dimension")
