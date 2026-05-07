package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"arkloop/services/shared/feishuclient"
	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
)

type feishuSenderCall struct {
	Kind          string
	ReceiveIDType string
	ReceiveID     string
	MessageID     string
	Text          string
	ReplyInThread bool
}

type fakeFeishuTextClient struct {
	calls          []feishuSenderCall
	sendMessageID  string
	replyMessageID string
}

func (f *fakeFeishuTextClient) SendText(ctx context.Context, receiveIDType, receiveID, text, uuid string) (*feishuclient.SentMessage, error) {
	f.calls = append(f.calls, feishuSenderCall{
		Kind:          "send",
		ReceiveIDType: receiveIDType,
		ReceiveID:     receiveID,
		Text:          text,
	})
	messageID := f.sendMessageID
	if messageID == "" {
		messageID = "om_send"
	}
	return &feishuclient.SentMessage{MessageID: messageID}, nil
}

func (f *fakeFeishuTextClient) ReplyText(ctx context.Context, messageID, text string, replyInThread bool, uuid string) (*feishuclient.SentMessage, error) {
	f.calls = append(f.calls, feishuSenderCall{
		Kind:          "reply",
		MessageID:     messageID,
		Text:          text,
		ReplyInThread: replyInThread,
	})
	outMessageID := f.replyMessageID
	if outMessageID == "" {
		outMessageID = "om_reply"
	}
	return &feishuclient.SentMessage{MessageID: outMessageID}, nil
}

func TestFeishuChannelSenderRepliesFirstSegmentThenSendsRestToChat(t *testing.T) {
	client := &fakeFeishuTextClient{}
	configJSON := []byte(`{"app_id":"cli_app","domain":"feishu"}`)
	sender := NewFeishuChannelSenderWithClient(client, configJSON, `{"app_secret":"app-secret"}`, 0)
	text := strings.Repeat("a", feishuMessageMaxLen) + "\nsecond"
	ids, err := sender.SendText(context.Background(), ChannelDeliveryTarget{
		ChannelType:  "feishu",
		Conversation: ChannelConversationRef{Target: "oc_chat"},
		ReplyTo:      &ChannelMessageRef{MessageID: "om_in"},
	}, text)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if strings.Join(ids, ",") != "om_reply,om_send" {
		t.Fatalf("unexpected message ids: %#v", ids)
	}
	if len(client.calls) != 2 {
		t.Fatalf("expected 2 message calls, got %d", len(client.calls))
	}
	if client.calls[0].Kind != "reply" || client.calls[0].MessageID != "om_in" {
		t.Fatalf("expected first call to reply, got %#v", client.calls[0])
	}
	if !client.calls[0].ReplyInThread {
		t.Fatalf("expected reply_in_thread=true")
	}
	if client.calls[0].Text != strings.Repeat("a", feishuMessageMaxLen) {
		t.Fatalf("unexpected first text length=%d", len([]rune(client.calls[0].Text)))
	}
	if client.calls[1].Kind != "send" || client.calls[1].ReceiveIDType != "chat_id" {
		t.Fatalf("unexpected direct call: %#v", client.calls[1])
	}
	if client.calls[1].ReceiveID != "oc_chat" {
		t.Fatalf("unexpected receive_id: %#v", client.calls[1].ReceiveID)
	}
	if client.calls[1].Text != "second" {
		t.Fatalf("unexpected second text: %q", client.calls[1].Text)
	}
}

func TestFeishuChannelSenderRejectsEmptyMessageID(t *testing.T) {
	client := &fakeFeishuTextClient{sendMessageID: " "}
	sender := NewFeishuChannelSenderWithClient(client, []byte(`{"app_id":"cli_app"}`), `{"app_secret":"app-secret"}`, 0)
	_, err := sender.SendText(context.Background(), ChannelDeliveryTarget{
		ChannelType:  "feishu",
		Conversation: ChannelConversationRef{Target: "oc_chat"},
	}, "hello")
	if err == nil || !strings.Contains(err.Error(), "message_id") {
		t.Fatalf("expected message_id error, got %v", err)
	}
}

func TestFeishuChannelSenderRequiresAppCredentials(t *testing.T) {
	cases := []struct {
		name       string
		configJSON []byte
		secret     string
		want       string
	}{
		{name: "missing app id", configJSON: []byte(`{"domain":"feishu"}`), secret: `{"app_secret":"secret"}`, want: "app_id"},
		{name: "missing app secret", configJSON: []byte(`{"app_id":"cli_app","domain":"feishu"}`), secret: " ", want: "app_secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender := NewFeishuChannelSender(tc.configJSON, tc.secret)
			_, err := sender.SendText(context.Background(), ChannelDeliveryTarget{
				ChannelType:  "feishu",
				Conversation: ChannelConversationRef{Target: "oc_chat"},
			}, "hello")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s error, got %v", tc.want, err)
			}
		})
	}
}

func TestBuildOutboxPayloadPreservesFeishuReplyRefs(t *testing.T) {
	accountID := uuid.New()
	runID := uuid.New()
	threadID := uuid.New()
	rc := &RunContext{
		Run: data.Run{ID: runID, AccountID: accountID, ThreadID: threadID},
		ChannelContext: &ChannelContext{
			ChannelType:      "feishu",
			ConversationType: "group",
			Conversation:     ChannelConversationRef{Target: "oc_chat"},
			InboundMessage:   ChannelMessageRef{MessageID: "om_in"},
			TriggerMessage:   &ChannelMessageRef{MessageID: "om_trigger"},
		},
	}

	payload := buildOutboxPayload(rc, "feishu", "hello", []string{"hello"}, false)
	if payload.ReplyToMessageID != "om_trigger" || payload.TriggerMessageID != "om_trigger" || payload.InboundMessageID != "om_in" {
		t.Fatalf("unexpected feishu reply refs: %#v", payload)
	}
	if payload.HeartbeatRun {
		t.Fatal("unexpected heartbeat flag")
	}

	rc.ChannelContext.ConversationType = "private"
	privatePayload := buildOutboxPayload(rc, "feishu", "hello", []string{"hello"}, false)
	if privatePayload.ReplyToMessageID != "" {
		t.Fatalf("expected no private reply ref, got %q", privatePayload.ReplyToMessageID)
	}
	if privatePayload.TriggerMessageID != "om_trigger" || privatePayload.InboundMessageID != "om_in" {
		t.Fatalf("expected private payload to preserve refs, got %#v", privatePayload)
	}

	rc.ChannelContext.ConversationType = "group"
	rc.HeartbeatRun = true
	heartbeatPayload := buildOutboxPayload(rc, "feishu", "hello", []string{"hello"}, false)
	if heartbeatPayload.ReplyToMessageID != "" || !heartbeatPayload.HeartbeatRun {
		t.Fatalf("unexpected heartbeat payload: %#v", heartbeatPayload)
	}
}

func feishuSenderTextContent(t *testing.T, raw any) string {
	t.Helper()
	textJSON, ok := raw.(string)
	if !ok {
		t.Fatalf("content is %T", raw)
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(textJSON), &content); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	return content.Text
}
