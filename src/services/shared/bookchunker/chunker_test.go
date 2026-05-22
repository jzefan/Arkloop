package bookchunker

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"arkloop/services/shared/bookparser"
)

const sampleParagraph = "光的干涉是指两列或多列频率相同的光波相遇时发生的现象，其结果是某些位置振幅相互加强，另一些位置振幅相互削弱，从而形成稳定的明暗条纹。1801 年托马斯·杨通过双缝实验首次证明了光具有波动性，实验中双缝间距、缝至屏的距离以及光的波长共同决定了条纹间距，可以由公式 Δy = λL/d 计算。"

func longParsedDoc() bookparser.ParsedDoc {
	blocks := []bookparser.Block{}
	for i := 0; i < 6; i++ {
		blocks = append(blocks, bookparser.Block{
			Type: bookparser.BlockParagraph,
			Text: sampleParagraph,
		})
	}
	return bookparser.ParsedDoc{Blocks: blocks}
}

func TestChunkLongParsedDocProducesMultipleChunks(t *testing.T) {
	chunks, err := Chunk(longParsedDoc(), DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.TokenCount > DefaultOptions().MaxTokens {
			t.Errorf("chunk %d exceeds MaxTokens: %d > %d", i, c.TokenCount, DefaultOptions().MaxTokens)
		}
		if c.Ordinal != i {
			t.Errorf("chunk %d ordinal: got %d", i, c.Ordinal)
		}
		if c.ChunkType != string(bookparser.BlockParagraph) {
			t.Errorf("chunk %d type: got %q", i, c.ChunkType)
		}
	}
}

func TestChunkShortParsedDocReturnsSingleChunk(t *testing.T) {
	doc := bookparser.ParsedDoc{Blocks: []bookparser.Block{{Type: bookparser.BlockParagraph, Text: "短句。"}}}
	chunks, err := Chunk(doc, DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Text != "短句。" {
		t.Errorf("unexpected: %+v", chunks)
	}
}

func TestChunkEmptyDocReturnsNil(t *testing.T) {
	chunks, _ := Chunk(bookparser.ParsedDoc{}, DefaultOptions())
	if chunks != nil {
		t.Errorf("expected nil, got %+v", chunks)
	}
}

func TestChunkPreservesHeadingPathFromBlock(t *testing.T) {
	doc := bookparser.ParsedDoc{Blocks: []bookparser.Block{
		{Type: bookparser.BlockParagraph, Text: "正文 A", HeadingPath: []string{"第一章"}},
		{Type: bookparser.BlockParagraph, Text: "正文 B", HeadingPath: []string{"第一章", "1.2 节"}},
	}}
	chunks, _ := Chunk(doc, DefaultOptions())
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if len(chunks[0].HeadingPath) != 1 || chunks[0].HeadingPath[0] != "第一章" {
		t.Errorf("chunk 0 heading: %+v", chunks[0].HeadingPath)
	}
	if len(chunks[1].HeadingPath) != 2 || chunks[1].HeadingPath[1] != "1.2 节" {
		t.Errorf("chunk 1 heading: %+v", chunks[1].HeadingPath)
	}
}

func TestChunkDeterministic(t *testing.T) {
	doc := longParsedDoc()
	a, _ := Chunk(doc, DefaultOptions())
	b, _ := Chunk(doc, DefaultOptions())
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Text != b[i].Text {
			t.Fatalf("chunk %d differs", i)
		}
	}
}

func TestChunkHandlesSpecialBlockAsSingleChunk(t *testing.T) {
	doc := bookparser.ParsedDoc{Blocks: []bookparser.Block{
		{Type: bookparser.BlockTable, Text: "| A | B |\n|---|---|\n| 1 | 2 |"},
	}}
	chunks, _ := Chunk(doc, DefaultOptions())
	if len(chunks) != 1 {
		t.Fatalf("got %d, want 1", len(chunks))
	}
	if chunks[0].ChunkType != string(bookparser.BlockTable) {
		t.Errorf("expected table type, got %q", chunks[0].ChunkType)
	}
}

func TestChunkProducesValidUTF8(t *testing.T) {
	buf, err := os.ReadFile("testdata/cn_textbook_excerpt.txt")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := bookparser.ParseText(strings.NewReader(string(buf)), "text/plain")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	chunks, err := Chunk(doc, DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	for i, chunk := range chunks {
		if !utf8.ValidString(chunk.Text) {
			t.Fatalf("chunk %d is not valid UTF-8: %q", i, chunk.Text)
		}
	}
}
