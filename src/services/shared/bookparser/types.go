// Package bookparser converts uploaded document bytes into a structured
// ParsedDoc that the chunker can consume. M1.0 ships text/plain and
// text/markdown only; PDF/DOCX implementations land in M1.1.
package bookparser

import (
	"context"
	"errors"
	"io"
)

// BlockType enumerates the kinds of content blocks a parser can emit.
type BlockType string

const (
	BlockParagraph BlockType = "paragraph"
	BlockHeading   BlockType = "heading"
	BlockImage     BlockType = "image"   // M1.1+
	BlockTable     BlockType = "table"   // M1.1+
	BlockFormula   BlockType = "formula" // M1.1+
)

// Block is one piece of source content.
type Block struct {
	Type              BlockType
	Text              string
	HeadingPath       []string
	HeadingInferred   bool
	HeadingConfidence float32
	Metadata          map[string]any
}

// ParsedDoc is the parser output.
type ParsedDoc struct {
	Blocks []Block
	Meta   map[string]any
}

// Parser turns raw bytes of a given mime into a ParsedDoc.
type Parser interface {
	Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error)
}

// ErrUnsupportedMime is returned when a parser receives a mime it can't handle.
var ErrUnsupportedMime = errors.New("bookparser: unsupported mime type")
