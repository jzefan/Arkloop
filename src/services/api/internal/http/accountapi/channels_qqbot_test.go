package accountapi

import (
	"testing"
	"time"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

func TestQQBotChannelDoesNotExposeWebhookURL(t *testing.T) {
	channelID := uuid.New()
	if got := buildWebhookURL("https://arkloop.example", "qqbot", channelID); got != "" {
		t.Fatalf("qqbot webhook URL = %q, want empty", got)
	}

	webhook := "https://arkloop.example/v1/channels/qqbot/" + channelID.String() + "/webhook"
	resp := toChannelResponse(data.Channel{
		ID:          channelID,
		AccountID:   uuid.New(),
		ChannelType: "qqbot",
		WebhookURL:  &webhook,
		ConfigJSON:  []byte(`{"app_id":"app-1"}`),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
	if resp.WebhookURL != nil {
		t.Fatalf("qqbot response webhook URL = %#v, want nil", resp.WebhookURL)
	}
}
