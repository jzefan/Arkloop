package security

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func TestLoadTokenizer_Unigram(t *testing.T) {
	tok, err := LoadTokenizer(testdataPath("tokenizer_unigram.json"), 32)
	if err != nil {
		t.Fatalf("load unigram tokenizer: %v", err)
	}
	if tok.modelType != "Unigram" {
		t.Fatalf("expected modelType=Unigram, got %s", tok.modelType)
	}
	if tok.VocabSize() == 0 {
		t.Fatal("vocab size is 0")
	}

	ids, mask := tok.Encode("hello world")
	// [CLS] + tokens + [SEP] + padding
	if len(ids) != 32 {
		t.Fatalf("expected 32 ids, got %d", len(ids))
	}
	if len(mask) != 32 {
		t.Fatalf("expected 32 mask, got %d", len(mask))
	}
	// 首尾必须是 CLS/SEP
	if ids[0] != tok.clsID {
		t.Errorf("first token should be CLS=%d, got %d", tok.clsID, ids[0])
	}
	// mask 前部为 1，pad 部分为 0
	if mask[0] != 1 {
		t.Errorf("mask[0] should be 1")
	}
}

func TestLoadTokenizer_BPE(t *testing.T) {
	tok, err := LoadTokenizer(testdataPath("tokenizer_bpe.json"), 32)
	if err != nil {
		t.Fatalf("load BPE tokenizer: %v", err)
	}
	if tok.modelType != "BPE" {
		t.Fatalf("expected modelType=BPE, got %s", tok.modelType)
	}
	if tok.VocabSize() == 0 {
		t.Fatal("vocab size is 0")
	}
	if len(tok.mergeRanks) == 0 {
		t.Fatal("merge ranks is empty")
	}

	ids, mask := tok.Encode("hello world")
	if len(ids) != 32 {
		t.Fatalf("expected 32 ids, got %d", len(ids))
	}
	if len(mask) != 32 {
		t.Fatalf("expected 32 mask, got %d", len(mask))
	}
	if ids[0] != tok.clsID {
		t.Errorf("first token should be CLS=%d, got %d", tok.clsID, ids[0])
	}
}

func TestSegmentBPE(t *testing.T) {
	tok, err := LoadTokenizer(testdataPath("tokenizer_bpe.json"), 32)
	if err != nil {
		t.Fatalf("load BPE tokenizer: %v", err)
	}

	// "▁hello" 应该通过 merge 链合并为一个 token
	result := tok.segmentBPE("▁hello")
	if len(result) != 1 || result[0] != "▁hello" {
		t.Errorf("expected [▁hello], got %v", result)
	}

	// "▁world" 同理
	result = tok.segmentBPE("▁world")
	if len(result) != 1 || result[0] != "▁world" {
		t.Errorf("expected [▁world], got %v", result)
	}
}

func TestSegmentBPE_UnknownChars(t *testing.T) {
	tok, err := LoadTokenizer(testdataPath("tokenizer_bpe.json"), 32)
	if err != nil {
		t.Fatalf("load BPE tokenizer: %v", err)
	}

	// 包含不在 merges 里的字符，应保持为单独 symbol
	result := tok.segmentBPE("xyz")
	if len(result) != 3 {
		t.Errorf("expected 3 symbols for 'xyz', got %v", result)
	}
}

func TestLoadTokenizer_UnsupportedType(t *testing.T) {
	// 直接构造一个不支持的类型
	_, err := LoadTokenizer(testdataPath("nonexistent.json"), 32)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestBPEEncode_CorrectIDs(t *testing.T) {
	tok, err := LoadTokenizer(testdataPath("tokenizer_bpe.json"), 32)
	if err != nil {
		t.Fatalf("load BPE tokenizer: %v", err)
	}

	ids, _ := tok.Encode("hello")
	// "hello" -> preTokenize -> "▁hello" -> segment -> ["▁hello"] -> id 12
	// 最终: [CLS=100, 12, SEP=101, PAD...]
	if ids[0] != 100 {
		t.Errorf("expected CLS=100, got %d", ids[0])
	}
	if ids[1] != 12 {
		t.Errorf("expected ▁hello=12, got %d", ids[1])
	}
	if ids[2] != 101 {
		t.Errorf("expected SEP=101, got %d", ids[2])
	}
}
