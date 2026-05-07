package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDuckduckgoProviderSearchParsesResponse(t *testing.T) {
	const payload = `{
  "Heading": "Topic",
  "Abstract": "Summary line",
  "AbstractURL": "https://example.com/a",
  "AbstractText": "",
  "RelatedTopics": [
    {"Text": "Second - snippet two", "FirstURL": "https://example.com/b"},
    {"Topics": [
      {"Text": "Nested - snip", "FirstURL": "https://example.com/c"}
    ]}
  ]
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			t.Fatal("expected format=json")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewDuckduckgoProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "q1", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3: %#v", len(got), got)
	}
	if got[0].URL != "https://example.com/a" || got[0].Title != "Topic" {
		t.Fatalf("first result: %#v", got[0])
	}
	if got[1].URL != "https://example.com/b" {
		t.Fatalf("second result: %#v", got[1])
	}
	if got[2].URL != "https://example.com/c" {
		t.Fatalf("third result: %#v", got[2])
	}
}

func TestDuckduckgoProviderParsesResultsArrayAndAbstractURLOnly(t *testing.T) {
	const payload = `{
  "AbstractURL": "https://en.wikipedia.org/wiki/Example",
  "Heading": "",
  "Abstract": "",
  "Results": [
    {"FirstURL": "https://www.example.com/", "Text": "Official site"}
  ],
  "RelatedTopics": []
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewDuckduckgoProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "q", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d %#v", len(got), got)
	}
	if got[0].URL != "https://en.wikipedia.org/wiki/Example" {
		t.Fatalf("first: %#v", got[0])
	}
	if got[1].URL != "https://www.example.com/" || got[1].Title != "Official site" {
		t.Fatalf("second: %#v", got[1])
	}
}

func TestDuckduckgoFallbackRetryUsesShorterQuery(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query().Get("q")
		switch q {
		case "one two three four five six seven":
			_, _ = w.Write([]byte(`{"AbstractURL":"","Results":[]}`))
		default:
			_, _ = w.Write([]byte(`{"Heading":"Wiki","AbstractURL":"https://wiki.example/","AbstractText":"body"}`))
		}
	}))
	t.Cleanup(srv.Close)

	p := NewDuckduckgoProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "one two three four five six seven", 3)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 HTTP calls, got %d", calls)
	}
	if len(got) != 1 || got[0].URL != "https://wiki.example/" {
		t.Fatalf("got %#v", got)
	}
}

func TestDuckduckgoProviderSearchRespectsMaxResults(t *testing.T) {
	const payload = `{
  "Heading": "H",
  "AbstractURL": "https://example.com/1",
  "AbstractText": "one",
  "RelatedTopics": [
    {"Text": "T2", "FirstURL": "https://example.com/2"},
    {"Text": "T3", "FirstURL": "https://example.com/3"}
  ]
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewDuckduckgoProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "q", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].URL != "https://example.com/1" {
		t.Fatalf("got %#v", got)
	}
}
