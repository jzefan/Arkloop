package bookparser

import (
	"io"
	"strings"
)

// ParseText reads UTF-8 text from r and splits on blank lines.
// Each non-empty paragraph becomes a BlockParagraph.
func ParseText(r io.Reader, mime string) (ParsedDoc, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return ParsedDoc{}, err
	}
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	parts := strings.Split(normalized, "\n\n")
	blocks := make([]Block, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		blocks = append(blocks, Block{Type: BlockParagraph, Text: t})
	}
	return ParsedDoc{
		Blocks: blocks,
		Meta: map[string]any{
			"source_mime": strings.SplitN(mime, ";", 2)[0],
			"byte_size":   len(raw),
		},
	}, nil
}
