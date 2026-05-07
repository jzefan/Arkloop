package pipeline

import (
	"context"
	"fmt"
	"strings"
)

type ChannelConversationRef struct {
	Target   string
	ThreadID *string
}

type ChannelMessageRef struct {
	MessageID string
}

type ChannelDeliveryTarget struct {
	ChannelType  string
	Conversation ChannelConversationRef
	ReplyTo      *ChannelMessageRef
	Metadata     map[string]any
}

type ChannelSender interface {
	SendText(ctx context.Context, target ChannelDeliveryTarget, text string) ([]string, error)
}

func requiredStringMapValue(values map[string]any, key string) (string, error) {
	raw, ok := values[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return strings.TrimSpace(text), nil
}

func optionalStringMapValue(values map[string]any, key string) (*string, error) {
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil, nil
	}
	text, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, nil
	}
	return &trimmed, nil
}
