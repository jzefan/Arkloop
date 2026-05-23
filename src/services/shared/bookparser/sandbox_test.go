package bookparser

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	req RunnerRequest
	out string
	err error
}

func (r *fakeRunner) Run(_ context.Context, req RunnerRequest) ([]byte, error) {
	r.req = req
	if r.err != nil {
		return nil, r.err
	}
	return []byte(r.out), nil
}

func TestSandboxParserDecodesRunnerJSON(t *testing.T) {
	runner := &fakeRunner{out: `{
		"meta":{"page_count":2,"heading_inferred_ratio":0.75},
		"blocks":[
			{"type":"heading","text":"第一章","heading_path":["第一章"],"heading_inferred":true,"heading_confidence":0.9,"metadata":{"page":1}},
			{"type":"image","text":"[Image: 光路图]","metadata":{"page":2,"asset_sha256":"abc"}}
		]
	}`}
	parser := NewSandboxParser(runner)

	doc, err := parser.Parse(context.Background(), strings.NewReader("pdf-bytes"), "application/pdf")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if runner.req.MIME != "application/pdf" || string(runner.req.Data) != "pdf-bytes" {
		t.Fatalf("runner req: %+v", runner.req)
	}
	if doc.Meta["source_mime"] != "application/pdf" || doc.Meta["byte_size"].(int) != len("pdf-bytes") {
		t.Fatalf("meta not enriched: %+v", doc.Meta)
	}
	if len(doc.Blocks) != 2 || doc.Blocks[1].Type != BlockImage {
		t.Fatalf("blocks: %+v", doc.Blocks)
	}
	if doc.Blocks[1].Metadata["asset_sha256"] != "abc" {
		t.Fatalf("metadata: %+v", doc.Blocks[1].Metadata)
	}
}

func TestSandboxParserRejectsUnsupportedMime(t *testing.T) {
	parser := NewSandboxParser(&fakeRunner{})
	_, err := parser.Parse(context.Background(), strings.NewReader("x"), "application/zip")
	if !errors.Is(err, ErrUnsupportedMime) {
		t.Fatalf("got %v, want ErrUnsupportedMime", err)
	}
}

func TestSandboxParserReturnsRunnerError(t *testing.T) {
	parser := NewSandboxParser(&fakeRunner{err: errors.New("boom")})
	_, err := parser.Parse(context.Background(), strings.NewReader("x"), "image/png")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
}
