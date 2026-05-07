package qqbotclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseCredentials(t *testing.T) {
	creds, err := ParseCredentials(json.RawMessage(`{"app_id":"app-1"}`), "secret-1")
	if err != nil {
		t.Fatalf("ParseCredentials: %v", err)
	}
	if creds.AppID != "app-1" || creds.ClientSecret != "secret-1" {
		t.Fatalf("unexpected credentials: %#v", creds)
	}

	creds, err = ParseCredentials(nil, "app-2:secret-2")
	if err != nil {
		t.Fatalf("ParseCredentials token pair: %v", err)
	}
	if creds.AppID != "app-2" || creds.ClientSecret != "secret-2" {
		t.Fatalf("unexpected token pair credentials: %#v", creds)
	}
}

func TestMessageCreateEventGroupSenderAndSelfMention(t *testing.T) {
	event := MessageCreateEvent{
		ID:      "msg-1",
		Content: "<@bot-openid> hello",
		Author: User{
			MemberOpenID: "member-openid",
			Username:     "Alice",
		},
		GroupOpenID: "group-openid",
		Mentions: []Mention{
			{MemberOpenID: "bot-openid", IsYou: true},
		},
	}

	if got := event.SenderOpenID(); got != "member-openid" {
		t.Fatalf("SenderOpenID = %q", got)
	}
	if got := event.SenderDisplayName(); got != "Alice" {
		t.Fatalf("SenderDisplayName = %q", got)
	}
	if got := event.ContentWithoutSelfMentions(); got != "hello" {
		t.Fatalf("ContentWithoutSelfMentions = %q", got)
	}
}

func TestDefaultIntentsOnlyRequestImplementedMessageEvents(t *testing.T) {
	if DefaultIntents != IntentC2CGroupAtMessages {
		t.Fatalf("DefaultIntents = %d, want %d", DefaultIntents, IntentC2CGroupAtMessages)
	}
}

func TestNewClientUsesDefaultHTTPTimeout(t *testing.T) {
	client := NewClient(Credentials{AppID: "app-1", ClientSecret: "secret-1"}, nil)
	if client.http == nil {
		t.Fatal("expected default http client")
	}
	if client.http.Timeout != defaultHTTPTimeout {
		t.Fatalf("default timeout = %s, want %s", client.http.Timeout, defaultHTTPTimeout)
	}

	custom := &http.Client{Timeout: time.Second}
	client = NewClient(Credentials{AppID: "app-1", ClientSecret: "secret-1"}, &ClientOptions{HTTPClient: custom})
	if client.http != custom {
		t.Fatal("expected custom http client")
	}
}

func TestClientTokenAndSendText(t *testing.T) {
	var tokenCalls int
	var sendPath string
	var sendAuth string
	var sendBodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode token body: %v", err)
			}
			if body["appId"] != "app-1" || body["clientSecret"] != "secret-1" {
				t.Fatalf("unexpected token body: %#v", body)
			}
			_, _ = w.Write([]byte(`{"access_token":"access-1","expires_in":"7200"}`))
		case "/v2/users/openid-1/messages":
			sendPath = r.URL.Path
			sendAuth = r.Header.Get("Authorization")
			var sendBody map[string]any
			if err := json.NewDecoder(r.Body).Decode(&sendBody); err != nil {
				t.Fatalf("decode send body: %v", err)
			}
			sendBodies = append(sendBodies, sendBody)
			_, _ = w.Write([]byte(`{"id":"msg-1"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient(Credentials{AppID: "app-1", ClientSecret: "secret-1"}, &ClientOptions{
		TokenURL: srv.URL + "/token",
		APIBase:  srv.URL,
	})
	resp, err := client.SendText(context.Background(), ScopeC2C, "openid-1", "hello", "inbound-1")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if resp.PlatformMessageID() != "msg-1" {
		t.Fatalf("message id = %q", resp.PlatformMessageID())
	}
	if tokenCalls != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls)
	}
	if sendPath != "/v2/users/openid-1/messages" {
		t.Fatalf("send path = %q", sendPath)
	}
	if sendAuth != "QQBot access-1" {
		t.Fatalf("authorization = %q", sendAuth)
	}
	if len(sendBodies) != 1 {
		t.Fatalf("send bodies = %d, want 1", len(sendBodies))
	}
	firstBody := sendBodies[0]
	if firstBody["content"] != "hello" || firstBody["msg_type"].(float64) != 0 || firstBody["msg_id"] != "inbound-1" {
		t.Fatalf("unexpected send body: %#v", firstBody)
	}
	firstSeq, ok := firstBody["msg_seq"].(float64)
	if !ok || firstSeq < 1 || firstSeq > 65535 {
		t.Fatalf("unexpected msg_seq: %#v", firstBody["msg_seq"])
	}

	if _, err := client.SendText(context.Background(), ScopeC2C, "openid-1", "hello again", "inbound-1"); err != nil {
		t.Fatalf("SendText second reply: %v", err)
	}
	if len(sendBodies) != 2 {
		t.Fatalf("send bodies = %d, want 2", len(sendBodies))
	}
	secondSeq, ok := sendBodies[1]["msg_seq"].(float64)
	if !ok || secondSeq < 1 || secondSeq > 65535 {
		t.Fatalf("unexpected second msg_seq: %#v", sendBodies[1]["msg_seq"])
	}
	if secondSeq == firstSeq {
		t.Fatalf("msg_seq repeated for same inbound message: %v", secondSeq)
	}
}

func TestClientSendTextGroupEndpoint(t *testing.T) {
	var sendPath string
	var sendBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"access-1","expires_in":7200}`))
		case "/v2/groups/group-openid/messages":
			sendPath = r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&sendBody); err != nil {
				t.Fatalf("decode send body: %v", err)
			}
			_, _ = w.Write([]byte(`{"id":"group-msg-1"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewClient(Credentials{AppID: "app-1", ClientSecret: "secret-1"}, &ClientOptions{
		TokenURL: srv.URL + "/token",
		APIBase:  srv.URL,
	})
	if _, err := client.SendText(context.Background(), ScopeGroup, "group-openid", "hello group", ""); err != nil {
		t.Fatalf("SendText group: %v", err)
	}
	if sendPath != "/v2/groups/group-openid/messages" {
		t.Fatalf("send path = %q", sendPath)
	}
	if _, ok := sendBody["msg_seq"].(float64); !ok {
		t.Fatalf("missing msg_seq in group body: %#v", sendBody)
	}
}
