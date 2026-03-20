package telegrambot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSendChatAction(t *testing.T) {
	t.Parallel()
	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	ctx := context.Background()
	err := c.SendChatAction(ctx, "TEST_TOKEN", SendChatActionRequest{ChatID: "123", Action: "typing"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/botTEST_TOKEN/sendChatAction") {
		t.Fatalf("path: %s", gotPath)
	}
	if !strings.Contains(string(gotBody), `"chat_id":"123"`) || !strings.Contains(string(gotBody), `"action":"typing"`) {
		t.Fatalf("body: %s", gotBody)
	}
}

func TestClientSetMessageReaction(t *testing.T) {
	t.Parallel()
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	ctx := context.Background()
	err := c.SetMessageReaction(ctx, "T", SetMessageReactionRequest{
		ChatID:    "9",
		MessageID: 42,
		Reaction:  []MessageReactionEmoji{{Type: "emoji", Emoji: "👍"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBody["chat_id"]) != `"9"` {
		t.Fatalf("chat_id: %s", gotBody["chat_id"])
	}
	if string(gotBody["message_id"]) != `42` {
		t.Fatalf("message_id: %s", gotBody["message_id"])
	}
}
