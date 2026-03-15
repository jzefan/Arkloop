//go:build !cgo

package security

import (
	"context"
	"fmt"
)

// SemanticScanner 无 CGO 环境下的占位实现。
type SemanticScanner struct{}

func NewSemanticScanner(cfg SemanticScannerConfig) (*SemanticScanner, error) {
	return nil, fmt.Errorf("semantic scanner requires CGO (onnxruntime)")
}

func (s *SemanticScanner) Classify(_ context.Context, text string) (SemanticResult, error) {
	return SemanticResult{}, fmt.Errorf("semantic scanner not available")
}

func (s *SemanticScanner) Close() {}
