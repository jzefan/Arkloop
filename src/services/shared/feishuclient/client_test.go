package feishuclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaseURL(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                           defaultFeishuBaseURL,
		"feishu":                     defaultFeishuBaseURL,
		"open.feishu.cn":             defaultFeishuBaseURL,
		" lark ":                     defaultLarkBaseURL,
		"https://open.larksuite.com": defaultLarkBaseURL,
		"https://example.com///":     defaultFeishuBaseURL,
	}
	for in, want := range cases {
		if got := BaseURL(in); got != want {
			t.Fatalf("BaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenCache(t *testing.T) {
	t.Parallel()
	tokenCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			tokenCalls++
			writeJSON(t, w, map[string]any{
				"code":                0,
				"msg":                 "ok",
				"tenant_access_token": "token-1",
				"expire":              7200,
			})
		case "/open-apis/bot/v3/info":
			if r.Header.Get("Authorization") != "Bearer token-1" {
				t.Fatalf("auth header: %q", r.Header.Get("Authorization"))
			}
			writeJSON(t, w, map[string]any{
				"code": 0,
				"msg":  "ok",
				"bot":  map[string]any{"app_name": "Arkloop"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{AppID: "app", AppSecret: "secret", baseURL: srv.URL}, srv.Client())
	for i := 0; i < 2; i++ {
		if _, err := c.GetBotInfo(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls)
	}
}

func TestSendText(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	var gotPath, gotQuery, gotAuth, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			writeToken(t, w)
		case "/open-apis/im/v1/messages":
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			gotAuth = r.Header.Get("Authorization")
			gotContentType = r.Header.Get("Content-Type")
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			writeJSON(t, w, map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{"message_id": "om_1", "chat_id": "oc_1", "msg_type": "text"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{AppID: "app", AppSecret: "secret", baseURL: srv.URL}, srv.Client())
	msg, err := c.SendText(context.Background(), "chat_id", "oc_1", "hello", "uuid-1")
	if err != nil {
		t.Fatal(err)
	}
	if msg.MessageID != "om_1" {
		t.Fatalf("message_id = %q", msg.MessageID)
	}
	if gotPath != "/open-apis/im/v1/messages" || gotQuery != "receive_id_type=chat_id" {
		t.Fatalf("target = %s?%s", gotPath, gotQuery)
	}
	if gotAuth != "Bearer token" {
		t.Fatalf("auth header: %q", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type: %q", gotContentType)
	}
	if gotBody["receive_id"] != "oc_1" || gotBody["msg_type"] != "text" || gotBody["uuid"] != "uuid-1" {
		t.Fatalf("body: %#v", gotBody)
	}
	assertContent(t, gotBody["content"], "hello")
}

func TestReplyText(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			writeToken(t, w)
		case "/open-apis/im/v1/messages/om_1/reply":
			gotPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			writeJSON(t, w, map[string]any{
				"code": 0,
				"msg":  "ok",
				"data": map[string]any{"message_id": "om_2"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{AppID: "app", AppSecret: "secret", baseURL: srv.URL}, srv.Client())
	msg, err := c.ReplyText(context.Background(), "om_1", "reply", true, "uuid-2")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/open-apis/im/v1/messages/om_1/reply" {
		t.Fatalf("path: %s", gotPath)
	}
	if msg.MessageID != "om_2" {
		t.Fatalf("message_id = %q", msg.MessageID)
	}
	if gotBody["msg_type"] != "text" || gotBody["uuid"] != "uuid-2" || gotBody["reply_in_thread"] != true {
		t.Fatalf("body: %#v", gotBody)
	}
	assertContent(t, gotBody["content"], "reply")
}

func TestSendTextRequiresMessageID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			writeToken(t, w)
		case "/open-apis/im/v1/messages":
			writeJSON(t, w, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{AppID: "app", AppSecret: "secret", baseURL: srv.URL}, srv.Client())
	_, err := c.SendText(context.Background(), "chat_id", "oc_1", "hello", "uuid-1")
	if err == nil || !strings.Contains(err.Error(), "message_id") {
		t.Fatalf("expected message_id error, got %v", err)
	}
}

func TestGetBotInfo(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			writeToken(t, w)
		case "/open-apis/bot/v3/info":
			writeJSON(t, w, map[string]any{
				"code": 0,
				"msg":  "ok",
				"bot": map[string]any{
					"app_name":      "Arkloop",
					"avatar_url":    "https://example.com/a.png",
					"ip_white_list": []string{"127.0.0.1"},
					"open_id":       "ou_1",
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{AppID: "app", AppSecret: "secret", baseURL: srv.URL}, srv.Client())
	info, err := c.GetBotInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.AppName != "Arkloop" || info.OpenID != "ou_1" || len(info.IPWhiteList) != 1 {
		t.Fatalf("bot info: %#v", info)
	}
}

func assertContent(t *testing.T, raw any, want string) {
	t.Helper()
	content, ok := raw.(string)
	if !ok {
		t.Fatalf("content is %T", raw)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	if parsed["text"] != want {
		t.Fatalf("content text = %q, want %q", parsed["text"], want)
	}
}

func writeToken(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	writeJSON(t, w, map[string]any{
		"code":                0,
		"msg":                 "ok",
		"tenant_access_token": "token",
		"expire":              7200,
	})
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
