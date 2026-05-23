package bookparser

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type parserFunc func(context.Context, io.Reader, string) (ParsedDoc, error)

func (f parserFunc) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error) {
	return f(ctx, r, mime)
}

func TestMultiFormatParserRoutesTextToTextParser(t *testing.T) {
	sandboxCalled := false
	p := NewMultiFormatParser(parserFunc(func(context.Context, io.Reader, string) (ParsedDoc, error) {
		sandboxCalled = true
		return ParsedDoc{}, nil
	}))

	doc, err := p.Parse(context.Background(), strings.NewReader("A\n\nB"), "text/markdown; charset=utf-8")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sandboxCalled {
		t.Fatal("sandbox parser should not be called for text")
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(doc.Blocks))
	}
}

func TestMultiFormatParserRoutesRichFormatsToSandbox(t *testing.T) {
	seen := map[string]bool{}
	p := NewMultiFormatParser(parserFunc(func(_ context.Context, _ io.Reader, mime string) (ParsedDoc, error) {
		seen[mime] = true
		return ParsedDoc{Blocks: []Block{{Type: BlockParagraph, Text: "ok"}}}, nil
	}))

	for _, mime := range []string{
		"application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"image/png",
		"image/jpeg",
		"image/webp",
	} {
		if _, err := p.Parse(context.Background(), strings.NewReader("x"), mime); err != nil {
			t.Fatalf("mime %s: %v", mime, err)
		}
	}
	for _, mime := range []string{"application/pdf", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "image/png", "image/jpeg", "image/webp"} {
		if !seen[mime] {
			t.Errorf("sandbox did not see %s", mime)
		}
	}
}

func TestMultiFormatParserRejectsUnsupportedMime(t *testing.T) {
	p := NewMultiFormatParser(parserFunc(func(context.Context, io.Reader, string) (ParsedDoc, error) {
		t.Fatal("sandbox parser should not be called")
		return ParsedDoc{}, nil
	}))
	_, err := p.Parse(context.Background(), strings.NewReader("x"), "application/zip")
	if !errors.Is(err, ErrUnsupportedMime) {
		t.Fatalf("got %v, want ErrUnsupportedMime", err)
	}
}
