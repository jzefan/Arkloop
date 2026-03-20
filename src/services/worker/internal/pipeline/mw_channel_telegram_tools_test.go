//go:build !desktop

package pipeline

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type stubTelegramToken struct{}

func (stubTelegramToken) BotToken(_ context.Context, _ uuid.UUID) (string, error) {
	return "test-token", nil
}

func TestChannelTelegramToolsMiddlewareInjectsTools(t *testing.T) {
	cid := uuid.New()
	ident := uuid.New()
	rc := &RunContext{
		JobPayload: map[string]any{
			"channel_delivery": map[string]any{
				"channel_id":                 cid.String(),
				"channel_type":               "telegram",
				"conversation_ref":           map[string]any{"target": "1"},
				"inbound_message_ref":        map[string]any{"message_id": "10"},
				"sender_channel_identity_id": ident.String(),
			},
		},
		ToolRegistry:  tools.NewRegistry(),
		ToolExecutors: map[string]tools.Executor{},
		AllowlistSet:  map[string]struct{}{},
		ToolSpecs:     nil,
		ToolDenylist:  nil,
	}

	h := Build([]RunMiddleware{
		NewChannelContextMiddleware(nil),
		NewChannelTelegramToolsMiddleware(stubTelegramToken{}, nil),
	}, func(_ context.Context, rc *RunContext) error {
		if _, ok := rc.AllowlistSet["telegram_react"]; !ok {
			t.Fatal("expected telegram_react in allowlist")
		}
		if _, ok := rc.AllowlistSet["telegram_reply"]; !ok {
			t.Fatal("expected telegram_reply in allowlist")
		}
		if rc.ToolExecutors["telegram_react"] == nil || rc.ToolExecutors["telegram_reply"] == nil {
			t.Fatal("expected executors bound")
		}
		if rc.ChannelToolSurface == nil || rc.ChannelToolSurface.PlatformChatID != "1" {
			t.Fatalf("channel surface: %#v", rc.ChannelToolSurface)
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestChannelTelegramToolsMiddlewareRespectsDenylist(t *testing.T) {
	cid := uuid.New()
	ident := uuid.New()
	rc := &RunContext{
		JobPayload: map[string]any{
			"channel_delivery": map[string]any{
				"channel_id":                 cid.String(),
				"channel_type":               "telegram",
				"conversation_ref":           map[string]any{"target": "1"},
				"inbound_message_ref":        map[string]any{"message_id": "10"},
				"sender_channel_identity_id": ident.String(),
			},
		},
		ToolRegistry:  tools.NewRegistry(),
		ToolExecutors: map[string]tools.Executor{},
		AllowlistSet:  map[string]struct{}{},
		ToolDenylist:  []string{"telegram_react"},
	}

	h := Build([]RunMiddleware{
		NewChannelContextMiddleware(nil),
		NewChannelTelegramToolsMiddleware(stubTelegramToken{}, nil),
	}, func(_ context.Context, rc *RunContext) error {
		if _, ok := rc.AllowlistSet["telegram_react"]; ok {
			t.Fatal("telegram_react should be denied")
		}
		if _, ok := rc.AllowlistSet["telegram_reply"]; !ok {
			t.Fatal("expected telegram_reply")
		}
		return nil
	})

	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("handler: %v", err)
	}
}
