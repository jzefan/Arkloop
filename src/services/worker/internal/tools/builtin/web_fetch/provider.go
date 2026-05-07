package webfetch

import (
	"context"
	"strings"
	"unicode/utf8"
)

type Result struct {
	URL          string
	RequestedURL string
	Title        string
	Content      string
	Truncated    bool
	StatusCode   int
	ContentType  string
	Extractor    string
	RawLength    int
}

func (r Result) ToJSON() map[string]any {
	content := strings.TrimSpace(r.Content)
	title := strings.TrimSpace(r.Title)
	rawLength := r.RawLength
	if rawLength <= 0 {
		rawLength = utf8.RuneCountInString(content)
	}
	payload := map[string]any{
		"url":        strings.TrimSpace(r.URL),
		"title":      title,
		"content":    content,
		"truncated":  r.Truncated,
		"length":     utf8.RuneCountInString(content),
		"raw_length": rawLength,
		"external_content": map[string]any{
			"source":    "web_fetch",
			"untrusted": true,
		},
	}
	if requested := strings.TrimSpace(r.RequestedURL); requested != "" && requested != strings.TrimSpace(r.URL) {
		payload["requested_url"] = requested
		payload["final_url"] = strings.TrimSpace(r.URL)
	}
	if r.StatusCode > 0 {
		payload["status_code"] = r.StatusCode
	}
	if contentType := strings.TrimSpace(r.ContentType); contentType != "" {
		payload["content_type"] = contentType
	}
	if extractor := strings.TrimSpace(r.Extractor); extractor != "" {
		payload["extractor"] = extractor
	}
	return payload
}

type Provider interface {
	Fetch(ctx context.Context, url string, maxLength int) (Result, error)
}

type HttpError struct {
	StatusCode int
}

func (e HttpError) Error() string {
	return "http error"
}

type UnsupportedContentTypeError struct {
	ContentType string
}

func (e UnsupportedContentTypeError) Error() string {
	contentType := strings.TrimSpace(e.ContentType)
	if contentType == "" {
		return "unsupported content type"
	}
	return "unsupported content type: " + contentType
}

func truncateString(value string, maxRunes int) (string, bool) {
	if maxRunes < 0 {
		maxRunes = 0
	}
	if utf8.RuneCountInString(value) <= maxRunes {
		return value, false
	}
	var builder strings.Builder
	builder.Grow(len(value))
	count := 0
	for _, r := range value {
		if count >= maxRunes {
			return builder.String(), true
		}
		builder.WriteRune(r)
		count++
	}
	return builder.String(), false
}
