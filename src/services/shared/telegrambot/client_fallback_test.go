package telegrambot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestClient_SendMessageWithHTMLFallback(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if calls.Load() == 1 {
			if !strings.Contains(string(body), `"parse_mode":"HTML"`) {
				t.Fatalf("first call missing HTML: %s", body)
			}
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: can't parse entities"}`))
			return
		}
		if strings.Contains(string(body), "parse_mode") {
			t.Fatalf("retry should omit parse_mode: %s", body)
		}
		if !strings.Contains(string(body), "hello") {
			t.Fatalf("expected plain text: %s", body)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	sent, err := c.SendMessageWithHTMLFallback(context.Background(), "tok", SendMessageRequest{
		ChatID:    "1",
		Text:      `<b>hello</b>`,
		ParseMode: ParseModeHTML,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent == nil || sent.MessageID != 7 {
		t.Fatalf("sent=%v", sent)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}
