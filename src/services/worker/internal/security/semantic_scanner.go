package security

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// SemanticResult 语义扫描结果。
type SemanticResult struct {
	Label       string  // "BENIGN", "INJECTION", "JAILBREAK"
	Score       float32
	IsInjection bool
}

// SemanticScanner 使用 ONNX Runtime 执行 Prompt Guard 模型推理。
type SemanticScanner struct {
	mu        sync.RWMutex
	session   *ort.DynamicAdvancedSession
	tokenizer *Tokenizer
	threshold float32
	labels    []string
}

// SemanticScannerConfig 创建 SemanticScanner 的配置。
type SemanticScannerConfig struct {
	ModelDir     string  // 包含 model.onnx 和 tokenizer.json 的目录
	OrtLibPath   string  // ONNX Runtime 共享库路径
	Threshold    float32 // 注入判定阈值，默认 0.5
	MaxSeqLen    int     // 最大序列长度，默认 512
}

var ortInitOnce sync.Once

// NewSemanticScanner 创建并初始化 SemanticScanner。
// 模型文件不存在时返回 error（调用方应降级为纯 Regex）。
func NewSemanticScanner(cfg SemanticScannerConfig) (*SemanticScanner, error) {
	modelPath := filepath.Join(cfg.ModelDir, "model.onnx")
	tokenizerPath := filepath.Join(cfg.ModelDir, "tokenizer.json")

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	if _, err := os.Stat(tokenizerPath); err != nil {
		return nil, fmt.Errorf("tokenizer not found: %w", err)
	}

	if cfg.OrtLibPath != "" {
		ort.SetSharedLibraryPath(cfg.OrtLibPath)
	}

	var initErr error
	ortInitOnce.Do(func() {
		initErr = ort.InitializeEnvironment()
	})
	if initErr != nil {
		return nil, fmt.Errorf("onnxruntime init: %w", initErr)
	}

	maxLen := cfg.MaxSeqLen
	if maxLen <= 0 {
		maxLen = 512
	}

	tokenizer, err := LoadTokenizer(tokenizerPath, maxLen)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	inputNames := []string{"input_ids", "attention_mask"}
	outputNames := []string{"logits"}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create onnx session: %w", err)
	}

	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 0.5
	}

	return &SemanticScanner{
		session:   session,
		tokenizer: tokenizer,
		threshold: threshold,
		labels:    []string{"BENIGN", "INJECTION", "JAILBREAK"},
	}, nil
}

// Classify 对文本进行语义分类。
func (s *SemanticScanner) Classify(text string) (SemanticResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.session == nil {
		return SemanticResult{}, fmt.Errorf("scanner not initialized")
	}

	inputIDs, attentionMask := s.tokenizer.Encode(text)

	seqLen := int64(len(inputIDs))
	shape := ort.Shape{1, seqLen}

	inputIDTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return SemanticResult{}, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer inputIDTensor.Destroy()

	maskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return SemanticResult{}, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	inputs := []ort.Value{inputIDTensor, maskTensor}
	outputs := []ort.Value{nil} // auto-allocate output

	if err := s.session.Run(inputs, outputs); err != nil {
		return SemanticResult{}, fmt.Errorf("onnx run: %w", err)
	}
	defer func() {
		for _, t := range outputs {
			if t != nil {
				t.Destroy()
			}
		}
	}()

	logitsTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return SemanticResult{}, fmt.Errorf("unexpected output tensor type")
	}

	logits := logitsTensor.GetData()
	probs := softmax(logits)

	bestIdx := 0
	bestScore := probs[0]
	for i := 1; i < len(probs); i++ {
		if probs[i] > bestScore {
			bestScore = probs[i]
			bestIdx = i
		}
	}

	label := "UNKNOWN"
	if bestIdx < len(s.labels) {
		label = s.labels[bestIdx]
	}

	return SemanticResult{
		Label:       label,
		Score:       bestScore,
		IsInjection: label != "BENIGN" && bestScore >= s.threshold,
	}, nil
}

// Close 释放 ONNX 会话。
func (s *SemanticScanner) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		s.session.Destroy()
		s.session = nil
	}
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
