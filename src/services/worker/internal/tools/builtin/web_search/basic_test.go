package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sharedoutbound "arkloop/services/shared/outboundurl"
)

func TestBasicProviderSearchParsesBingHTML(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	redirectTarget := "https://example.com/b"
	redirectValue := "a1aHR0cHM6Ly9leGFtcGxlLmNvbS9i"
	payload := `<html><body>
<ol id="b_results">
  <li class="b_algo">
    <h2><a href="https://example.com/a">Arkloop Search Example A</a></h2>
    <div class="b_caption"><p>Snippet A</p></div>
  </li>
  <li class="b_algo">
    <h2><a href="https://www.bing.com/ck/a?u=` + redirectValue + `">Arkloop Search Example B</a></h2>
    <div class="b_caption"><p>Snippet B</p></div>
  </li>
  <li class="b_algo">
    <h2><a href="https://example.com/a#section">Duplicate A</a></h2>
    <p>Duplicate snippet</p>
  </li>
</ol>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Fatalf("expected /search path, got %q", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "arkloop search" {
			t.Fatalf("query: %q", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("count") != "3" {
			t.Fatalf("count: %q", r.URL.Query().Get("count"))
		}
		if r.URL.Query().Get("mkt") != "en-US" || r.URL.Query().Get("setlang") != "en-US" {
			t.Fatalf("locale: mkt=%q setlang=%q", r.URL.Query().Get("mkt"), r.URL.Query().Get("setlang"))
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "ArkloopWebSearch") {
			t.Fatalf("user-agent: %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewBasicProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "arkloop search", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	if got[0].Title != "Arkloop Search Example A" || got[0].URL != "https://example.com/a" || got[0].Snippet != "Snippet A" {
		t.Fatalf("first result: %#v", got[0])
	}
	if got[1].Title != "Arkloop Search Example B" || got[1].URL != redirectTarget || got[1].Snippet != "Snippet B" {
		t.Fatalf("second result: %#v", got[1])
	}
}

func TestBasicProviderSearchUsesQueryMarket(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	const payload = `<html><body><ol id="b_results">
  <li class="b_algo">
    <h2><a href="https://example.cn/news">小米技术新闻</a></h2>
    <p>中文摘要</p>
  </li>
</ol></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("mkt") != "zh-CN" || r.URL.Query().Get("setlang") != "zh-CN" {
			t.Fatalf("locale: mkt=%q setlang=%q", r.URL.Query().Get("mkt"), r.URL.Query().Get("setlang"))
		}
		if got := r.Header.Get("Accept-Language"); !strings.HasPrefix(got, "zh-CN") {
			t.Fatalf("accept-language: %q", got)
		}
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewBasicProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "小米最新技术突破 2025", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://example.cn/news" {
		t.Fatalf("got %#v", got)
	}
}

func TestBasicProviderSearchFallbackStaysInsideResults(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	const payload = `<html><body>
<main>
  <a href="https://support.microsoft.com/contact">Contact Us - Microsoft Support</a>
  <ol id="b_results">
    <li>
      <h2><a href="https://docs.example.com/page">Docs Example</a></h2>
      <p>Fallback snippet</p>
    </li>
  </ol>
</main>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewBasicProviderWithBaseURL(srv.URL)
	got, err := p.Search(context.Background(), "docs", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}
	if got[0].Title != "Docs Example" || got[0].URL != "https://docs.example.com/page" || got[0].Snippet != "Fallback snippet" {
		t.Fatalf("result: %#v", got[0])
	}
}

func TestBasicProviderSearchRejectsBlockedPageWithoutResults(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	const payload = `<html><body>
<script>var CfConfig = { siteKey: "0x4AAAAA" };</script>
<iframe src="https://challenges.cloudflare.com/turnstile/v0/b/"></iframe>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)

	p := NewBasicProviderWithBaseURL(srv.URL)
	_, err := p.Search(context.Background(), "arkloop search", 5)
	if err == nil {
		t.Fatal("expected challenge error")
	}
	if !strings.Contains(err.Error(), "challenge page") {
		t.Fatalf("error: %v", err)
	}
}

func TestBasicProviderSearchHTTPError(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	p := NewBasicProviderWithBaseURL(srv.URL)
	_, err := p.Search(context.Background(), "q", 1)
	if err == nil {
		t.Fatal("expected error")
	}
	httpErr, ok := err.(HttpError)
	if !ok {
		t.Fatalf("expected HttpError, got %T %v", err, err)
	}
	if httpErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status: %d", httpErr.StatusCode)
	}
}
