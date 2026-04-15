package pipeline

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/llm"
)

func TestNewChannelGroupContextTrimMiddleware_projectsButSkipsTrimForPrivate(t *testing.T) {
	mw := NewChannelGroupContextTrimMiddleware()
	rc := &RunContext{
		ChannelContext: &ChannelContext{ConversationType: "private"},
		Messages: []llm.Message{{
			Role: "user",
			Content: []llm.ContentPart{{
				Type: "text",
				Text: "---\ndisplay-name: \"Alice\"\nchannel: \"telegram\"\nconversation-type: \"private\"\ntime: \"2026-04-03T10:00:00Z\"\n---\nhello",
			}},
		}},
	}
	called := false
	err := mw(context.Background(), rc, func(context.Context, *RunContext) error {
		called = true
		if len(rc.Messages) != 1 {
			t.Fatalf("messages should not be trimmed for DM")
		}
		text := llm.PartPromptText(rc.Messages[0].Content[0])
		if text == "" || text == "---\ndisplay-name: \"Alice\"\nchannel: \"telegram\"\nconversation-type: \"private\"\ntime: \"2026-04-03T10:00:00Z\"\n---\nhello" {
			t.Fatalf("expected projected envelope text, got %q", text)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("next not invoked")
	}
}

func TestNewChannelGroupContextTrimMiddleware_skipsProjectionWithoutChannelContext(t *testing.T) {
	mw := NewChannelGroupContextTrimMiddleware()
	original := "---\ndisplay-name: \"Alice\"\nchannel: \"telegram\"\nconversation-type: \"private\"\ntime: \"2026-04-03T10:00:00Z\"\n---\nhello"
	rc := &RunContext{
		Messages: []llm.Message{{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: original}}}},
	}

	_ = mw(context.Background(), rc, func(context.Context, *RunContext) error { return nil })

	if got := llm.PartPromptText(rc.Messages[0].Content[0]); got != original {
		t.Fatalf("expected envelope to stay untouched without channel context, got %q", got)
	}
}

func TestNewChannelGroupContextTrimMiddleware_preservesSupergroupHistory(t *testing.T) {
	mw := NewChannelGroupContextTrimMiddleware()
	long := "wwwwwwwwwwwwwwwwwwww"
	msgs := []llm.Message{
		{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: long}}},
		{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: long}}},
		{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "tail"}}},
	}
	rc := &RunContext{
		ChannelContext: &ChannelContext{ConversationType: "supergroup"},
		Messages:       msgs,
	}
	_ = mw(context.Background(), rc, func(context.Context, *RunContext) error { return nil })
	if len(rc.Messages) != len(msgs) {
		t.Fatalf("expected group middleware to preserve history, got %d", len(rc.Messages))
	}
	if rc.Messages[len(rc.Messages)-1].Content[0].Text != "tail" {
		t.Fatalf("unexpected tail content")
	}
}

func TestNewChannelGroupContextTrimMiddleware_preservesReplacementPrefix(t *testing.T) {
	mw := NewChannelGroupContextTrimMiddleware()
	rc := &RunContext{
		ChannelContext: &ChannelContext{ConversationType: "supergroup"},
		Messages: []llm.Message{
			makeThreadContextReplacementMessage("summary"),
			{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "body"}}},
			{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "tail"}}},
		},
	}
	_ = mw(context.Background(), rc, func(context.Context, *RunContext) error { return nil })
	if len(rc.Messages) != 3 {
		t.Fatalf("expected replacement and raw history preserved, got %d", len(rc.Messages))
	}
	if got := rc.Messages[0].Role; got != "system" {
		t.Fatalf("expected replacement to stay as system block, got %q", got)
	}
}
