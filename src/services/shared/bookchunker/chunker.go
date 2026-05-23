// Package bookchunker splits a ParsedDoc into overlapping chunks suitable
// for embedding-based retrieval. Each input Block becomes one or more
// output Chunks; special blocks (image/table/formula) are emitted as a
// single chunk each, preserving their type and metadata.
//
// Pure function: no I/O, no goroutines, deterministic given the same input.
package bookchunker

import (
	"fmt"
	"strings"

	"arkloop/services/shared/bookparser"

	"github.com/pkoukk/tiktoken-go"
)

// TextChunk is one output unit. Ordinal is 0-based across the whole document.
type TextChunk struct {
	Ordinal     int
	ChunkType   string
	Text        string
	HeadingPath []string
	TokenCount  int
	Metadata    map[string]any
}

type ChunkOptions struct {
	MinTokens     int
	MaxTokens     int
	OverlapTokens int
	Encoding      string
}

func DefaultOptions() ChunkOptions {
	return ChunkOptions{
		MinTokens:     256,
		MaxTokens:     512,
		OverlapTokens: 40,
		Encoding:      "cl100k_base",
	}
}

// Chunk transforms doc into a sequence of chunks per opts. Empty doc returns nil.
func Chunk(doc bookparser.ParsedDoc, opts ChunkOptions) ([]TextChunk, error) {
	if len(doc.Blocks) == 0 {
		return nil, nil
	}
	opts = normalizeOptions(opts)
	enc, err := tiktoken.GetEncoding(opts.Encoding)
	if err != nil && opts.Encoding != DefaultOptions().Encoding {
		return nil, fmt.Errorf("load tiktoken encoding %q: %w", opts.Encoding, err)
	}

	var out []TextChunk
	keepHeadingPath := keepInferredHeadingPath(doc.Meta)
	for _, block := range doc.Blocks {
		blockType := normalizeBlockType(block.Type)
		headingPath := copyPath(block.HeadingPath)
		if !keepHeadingPath {
			headingPath = nil
		}
		if blockType != bookparser.BlockParagraph && blockType != bookparser.BlockHeading {
			tokenCount := countTokens(enc, block.Text)
			out = append(out, TextChunk{
				Ordinal:     len(out),
				ChunkType:   string(blockType),
				Text:        block.Text,
				HeadingPath: headingPath,
				TokenCount:  tokenCount,
				Metadata:    copyMetadata(block.Metadata),
			})
			continue
		}

		tokenCount := countTokens(enc, block.Text)
		if tokenCount <= opts.MaxTokens {
			out = append(out, TextChunk{
				Ordinal:     len(out),
				ChunkType:   string(blockType),
				Text:        block.Text,
				HeadingPath: headingPath,
				TokenCount:  tokenCount,
				Metadata:    copyMetadata(block.Metadata),
			})
			continue
		}

		for _, segment := range splitLongText(enc, block.Text, opts) {
			out = append(out, TextChunk{
				Ordinal:     len(out),
				ChunkType:   string(blockType),
				Text:        segment.Text,
				HeadingPath: headingPath,
				TokenCount:  segment.TokenCount,
				Metadata:    copyMetadata(block.Metadata),
			})
		}
	}
	return out, nil
}

func normalizeOptions(opts ChunkOptions) ChunkOptions {
	defaults := DefaultOptions()
	if opts.Encoding == "" {
		opts.Encoding = defaults.Encoding
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = defaults.MaxTokens
	}
	if opts.MinTokens <= 0 {
		opts.MinTokens = defaults.MinTokens
	}
	if opts.OverlapTokens < 0 {
		opts.OverlapTokens = 0
	}
	return opts
}

func normalizeBlockType(t bookparser.BlockType) bookparser.BlockType {
	switch t {
	case bookparser.BlockParagraph, bookparser.BlockHeading, bookparser.BlockImage, bookparser.BlockTable, bookparser.BlockFormula:
		return t
	default:
		return bookparser.BlockParagraph
	}
}

func copyPath(p []string) []string {
	if len(p) == 0 {
		return nil
	}
	cp := make([]string, len(p))
	copy(cp, p)
	return cp
}

func copyMetadata(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func keepInferredHeadingPath(meta map[string]any) bool {
	if meta == nil {
		return true
	}
	raw, ok := meta["heading_inferred_ratio"]
	if !ok {
		return true
	}
	switch v := raw.(type) {
	case float64:
		return v >= 0.5
	case float32:
		return v >= 0.5
	case int:
		return float64(v) >= 0.5
	case int64:
		return float64(v) >= 0.5
	default:
		return true
	}
}

func safeDecode(enc *tiktoken.Tiktoken, tokens []int) string {
	return strings.ToValidUTF8(enc.Decode(tokens), "")
}

type textSegment struct {
	Text       string
	TokenCount int
}

func countTokens(enc *tiktoken.Tiktoken, text string) int {
	if enc == nil {
		return len([]rune(text))
	}
	return len(enc.Encode(text, nil, nil))
}

func splitLongText(enc *tiktoken.Tiktoken, text string, opts ChunkOptions) []textSegment {
	if enc == nil {
		return splitRunes(text, opts)
	}
	tokens := enc.Encode(text, nil, nil)
	var out []textSegment
	pos := 0
	for pos < len(tokens) {
		end := pos + opts.MaxTokens
		if end > len(tokens) {
			end = len(tokens)
		}
		window := tokens[pos:end]
		out = append(out, textSegment{Text: safeDecode(enc, window), TokenCount: len(window)})
		if end == len(tokens) {
			break
		}
		pos += chunkStep(opts)
	}
	return out
}

func splitRunes(text string, opts ChunkOptions) []textSegment {
	runes := []rune(text)
	var out []textSegment
	pos := 0
	for pos < len(runes) {
		end := pos + opts.MaxTokens
		if end > len(runes) {
			end = len(runes)
		}
		window := runes[pos:end]
		out = append(out, textSegment{Text: string(window), TokenCount: len(window)})
		if end == len(runes) {
			break
		}
		pos += chunkStep(opts)
	}
	return out
}

func chunkStep(opts ChunkOptions) int {
	step := opts.MaxTokens - opts.OverlapTokens
	if step < opts.MinTokens {
		step = opts.MinTokens
	}
	return step
}
