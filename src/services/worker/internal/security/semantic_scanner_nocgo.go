//go:build !cgo

package security

import "fmt"

// SemanticScanner 无 CGO 环境下的占位实现。
type SemanticScanner struct{}

func NewSemanticScanner(cfg SemanticScannerConfig) (*SemanticScanner, error) {
	return nil, fmt.Errorf("semantic scanner requires CGO (onnxruntime)")
}

func (s *SemanticScanner) Classify(text string) (SemanticResult, error) {
	return SemanticResult{}, fmt.Errorf("semantic scanner not available")
}

func (s *SemanticScanner) Close() {}
