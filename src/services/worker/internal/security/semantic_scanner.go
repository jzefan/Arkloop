package security

import "math"

// SemanticResult 语义扫描结果。
type SemanticResult struct {
	Label       string  // "BENIGN", "INJECTION", "JAILBREAK"
	Score       float32
	IsInjection bool
}

// SemanticScannerConfig 创建 SemanticScanner 的配置。
type SemanticScannerConfig struct {
	ModelDir   string  // 包含 model.onnx 和 tokenizer.json 的目录
	OrtLibPath string  // ONNX Runtime 共享库路径
	Threshold  float32 // 注入判定阈值，默认 0.5
	MaxSeqLen  int     // 最大序列长度，默认 512
}

func softmax(logits []float32) []float32 {
	max := logits[0]
	for _, v := range logits[1:] {
		if v > max {
			max = v
		}
	}
	sum := float32(0)
	probs := make([]float32, len(logits))
	for i, v := range logits {
		probs[i] = float32(math.Exp(float64(v - max)))
		sum += probs[i]
	}
	for i := range probs {
		probs[i] /= sum
	}
	return probs
}
