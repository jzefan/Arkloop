package bookparser

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// TextOnlyParser handles text/plain and text/markdown. Multi-format parsing
// will wrap additional backends in later milestones.
type TextOnlyParser struct{}

// NewTextOnlyParser returns the M1.0 default parser.
func NewTextOnlyParser() *TextOnlyParser { return &TextOnlyParser{} }

// Parse implements Parser. Only text/plain and text/markdown with optional
// charset params are accepted; anything else returns ErrUnsupportedMime.
func (p *TextOnlyParser) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error) {
	_ = ctx
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch base {
	case "text/plain", "text/markdown":
		return ParseText(r, mime)
	default:
		return ParsedDoc{}, fmt.Errorf("%w: %s", ErrUnsupportedMime, mime)
	}
}
