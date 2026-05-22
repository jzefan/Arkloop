package bookparser

import (
	"strings"
	"testing"
)

func TestTextParserSplitsByBlankLines(t *testing.T) {
	in := "段落 A 内容。\n\n段落 B 内容。\n\n段落 C。"
	doc, err := ParseText(strings.NewReader(in), "text/plain")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("blocks: got %d, want 3", len(doc.Blocks))
	}
	for i, b := range doc.Blocks {
		if b.Type != BlockParagraph {
			t.Errorf("block %d type: got %q, want paragraph", i, b.Type)
		}
		if b.HeadingInferred {
			t.Errorf("block %d: HeadingInferred should be false", i)
		}
	}
}

func TestTextParserSkipsEmptyParagraphs(t *testing.T) {
	in := "\n\nA\n\n\n\n\n\nB\n\n"
	doc, _ := ParseText(strings.NewReader(in), "text/plain")
	if len(doc.Blocks) != 2 {
		t.Errorf("got %d blocks, want 2", len(doc.Blocks))
	}
}

func TestTextParserHandlesCRLF(t *testing.T) {
	in := "A\r\n\r\nB"
	doc, _ := ParseText(strings.NewReader(in), "text/plain")
	if len(doc.Blocks) != 2 || doc.Blocks[0].Text != "A" || doc.Blocks[1].Text != "B" {
		t.Errorf("CRLF handling: %+v", doc.Blocks)
	}
}

func TestTextParserMetadataPopulated(t *testing.T) {
	in := "hello world"
	doc, _ := ParseText(strings.NewReader(in), "text/plain")
	if doc.Meta["source_mime"] != "text/plain" {
		t.Errorf("source_mime: %v", doc.Meta["source_mime"])
	}
}

func TestParserDispatchUnsupportedMime(t *testing.T) {
	p := NewTextOnlyParser()
	_, err := p.Parse(nil, strings.NewReader("x"), "application/pdf")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected unsupported-mime error, got %v", err)
	}
}

func TestParserDispatchAcceptsTextPlainAndMarkdown(t *testing.T) {
	p := NewTextOnlyParser()
	for _, mime := range []string{"text/plain", "text/markdown", "text/markdown; charset=utf-8"} {
		if _, err := p.Parse(nil, strings.NewReader("A\n\nB"), mime); err != nil {
			t.Errorf("mime %s rejected: %v", mime, err)
		}
	}
}
