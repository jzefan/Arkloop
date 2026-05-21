package bookchunker

import (
	"strings"
	"testing"
)

// 中文教科书片段，约 2000 字符，3 个段落（两个空行分隔）。
const sampleParagraph = "光的干涉是指两列或多列频率相同的光波相遇时发生的现象，其结果是某些位置振幅相互加强，另一些位置振幅相互削弱，从而形成稳定的明暗条纹。1801 年托马斯·杨通过双缝实验首次证明了光具有波动性，实验中双缝间距、缝至屏的距离以及光的波长共同决定了条纹间距，可以由公式 Δy = λL/d 计算。"

func TestChunkSinglePassParagraph(t *testing.T) {
	text := strings.Repeat(sampleParagraph, 6) // ~5–6k Chinese chars => > 512 tokens
	chunks, err := Chunk(text, DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks for long input, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.TokenCount > DefaultOptions().MaxTokens {
			t.Errorf("chunk %d exceeds MaxTokens: got %d > %d", i, c.TokenCount, DefaultOptions().MaxTokens)
		}
		if c.TokenCount < DefaultOptions().MinTokens && i != len(chunks)-1 {
			t.Errorf("non-tail chunk %d below MinTokens: got %d < %d", i, c.TokenCount, DefaultOptions().MinTokens)
		}
		if c.Text == "" {
			t.Errorf("chunk %d empty", i)
		}
		if c.Ordinal != i {
			t.Errorf("chunk %d ordinal mismatch: got %d", i, c.Ordinal)
		}
	}
}

func TestChunkOverlap(t *testing.T) {
	text := strings.Repeat(sampleParagraph, 6)
	chunks, _ := Chunk(text, DefaultOptions())
	if len(chunks) < 2 {
		t.Skip("need >=2 chunks for overlap test")
	}
	// Last 20 runes of chunk N should appear in chunk N+1's first 80 runes (loose overlap check).
	prevTail := []rune(chunks[0].Text)
	if len(prevTail) > 30 {
		prevTail = prevTail[len(prevTail)-30:]
	}
	head := []rune(chunks[1].Text)
	if len(head) > 120 {
		head = head[:120]
	}
	if !strings.Contains(string(head), string(prevTail[len(prevTail)/2:])) {
		t.Errorf("expected overlap between chunk 0 tail and chunk 1 head")
	}
}

func TestChunkShortInputReturnsSingleChunk(t *testing.T) {
	chunks, err := Chunk("短句。", DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short input, got %d", len(chunks))
	}
	if chunks[0].Text != "短句。" {
		t.Errorf("unexpected text: %q", chunks[0].Text)
	}
}

func TestChunkEmptyInputReturnsEmpty(t *testing.T) {
	chunks, err := Chunk("", DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestChunkDeterministic(t *testing.T) {
	text := strings.Repeat(sampleParagraph, 6)
	a, _ := Chunk(text, DefaultOptions())
	b, _ := Chunk(text, DefaultOptions())
	if len(a) != len(b) {
		t.Fatalf("non-deterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Text != b[i].Text || a[i].TokenCount != b[i].TokenCount {
			t.Fatalf("chunk %d differs across runs", i)
		}
	}
}
