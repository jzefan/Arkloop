package bookparser

import (
	"context"
	"fmt"
	"io"
	"strings"
)

var sandboxMIMEs = map[string]struct{}{
	"application/pdf":    {},
	"application/x-pdf":  {},
	"application/msword": {},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
	"image/png":  {},
	"image/jpeg": {},
	"image/jpg":  {},
	"image/webp": {},
}

// MultiFormatParser dispatches text formats to the in-process parser and
// rich document/image formats to a sandbox-backed parser.
type MultiFormatParser struct {
	text    Parser
	sandbox Parser
}

func NewMultiFormatParser(sandbox Parser) *MultiFormatParser {
	if sandbox == nil {
		sandbox = NewSandboxParser(DefaultLocalRunner())
	}
	return &MultiFormatParser{text: NewTextOnlyParser(), sandbox: sandbox}
}

func (p *MultiFormatParser) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error) {
	base := NormalizeMIME(mime)
	switch base {
	case "text/plain", "text/markdown":
		return p.text.Parse(ctx, r, base)
	default:
		if _, ok := sandboxMIMEs[base]; ok {
			return p.sandbox.Parse(ctx, r, base)
		}
		return ParsedDoc{}, fmt.Errorf("%w: %s", ErrUnsupportedMime, mime)
	}
}

func NormalizeMIME(mime string) string {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch base {
	case "text/x-markdown", "application/markdown":
		return "text/markdown"
	case "image/jpg":
		return "image/jpeg"
	default:
		return base
	}
}
