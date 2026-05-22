// Package bookchunker splits long text into overlapping chunks suitable for
// embedding-based retrieval. Pure function; no I/O. M0 inputs are paragraph-
// separated plain text; M1 will extend the signature with a structured
// ParsedDoc input but keep the chunk output shape stable.
package bookchunker

import (
	"fmt"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// TextChunk is one output unit. Ordinal is 0-based source order.
type TextChunk struct {
	Ordinal    int
	Text       string
	TokenCount int
}

// ChunkOptions tunes split behavior. Use DefaultOptions for M0.
type ChunkOptions struct {
	MinTokens     int
	MaxTokens     int
	OverlapTokens int
	Encoding      string // tiktoken encoding name; cl100k_base is the M0 default
}

// DefaultOptions returns the M0 default parameters.
func DefaultOptions() ChunkOptions {
	return ChunkOptions{
		MinTokens:     256,
		MaxTokens:     512,
		OverlapTokens: 40,
		Encoding:      "cl100k_base",
	}
}

// Chunk splits text into chunks per opts. Empty input returns nil.
func ChunkText(text string, opts ChunkOptions) ([]TextChunk, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	if opts.Encoding == "" {
		opts.Encoding = DefaultOptions().Encoding
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = DefaultOptions().MaxTokens
	}
	if opts.MinTokens <= 0 {
		opts.MinTokens = DefaultOptions().MinTokens
	}
	if opts.OverlapTokens < 0 {
		opts.OverlapTokens = 0
	}
	enc, err := tiktoken.GetEncoding(opts.Encoding)
	if err != nil {
		return nil, fmt.Errorf("load tiktoken encoding %q: %w", opts.Encoding, err)
	}

	paragraphs := splitParagraphs(text)
	tokens := make([][]int, len(paragraphs))
	totalTokens := 0
	for i, p := range paragraphs {
		tokens[i] = enc.Encode(p, nil, nil)
		totalTokens += len(tokens[i])
	}
	if totalTokens <= opts.MaxTokens {
		return []TextChunk{{
			Ordinal:    0,
			Text:       strings.Join(paragraphs, "\n\n"),
			TokenCount: totalTokens,
		}}, nil
	}

	flat := make([]int, 0, totalTokens)
	for _, t := range tokens {
		flat = append(flat, t...)
	}

	var chunks []TextChunk
	pos := 0
	for pos < len(flat) {
		end := pos + opts.MaxTokens
		if end > len(flat) {
			end = len(flat)
		}
		window := flat[pos:end]
		chunks = append(chunks, TextChunk{
			Ordinal:    len(chunks),
			Text:       enc.Decode(window),
			TokenCount: len(window),
		})
		if end == len(flat) {
			break
		}
		step := opts.MaxTokens - opts.OverlapTokens
		if step < opts.MinTokens {
			step = opts.MinTokens
		}
		pos += step
	}
	return chunks, nil
}

// Chunk splits text into chunks per opts. Empty input returns nil.
func Chunk(text string, opts ChunkOptions) ([]TextChunk, error) {
	return ChunkText(text, opts)
}

// splitParagraphs splits on blank lines, trimming each paragraph.
func splitParagraphs(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
