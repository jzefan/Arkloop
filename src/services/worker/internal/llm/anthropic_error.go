package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

func anthropicErrorMessageAndDetails(body []byte, status int) (string, map[string]any) {
	details := map[string]any{"status_code": status}
	fallback := fmt.Sprintf("Anthropic request failed: status=%d", status)

	if len(body) > 0 {
		raw, truncated := truncateUTF8(string(body), anthropicMaxErrorBodyBytes)
		details["provider_error_body"] = raw
		if truncated {
			details["provider_error_body_truncated"] = true
		}
	}

	if len(body) == 0 {
		return fallback, details
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fallback, details
	}
	root, ok := parsed.(map[string]any)
	if !ok {
		return fallback, details
	}
	errObj, ok := root["error"].(map[string]any)
	if !ok && anthropicErrorObject(root) {
		errObj = root
		ok = true
	}
	if !ok {
		if msg, ok := root["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg), details
		}
		return fallback, details
	}
	if errType, ok := errObj["type"].(string); ok && strings.TrimSpace(errType) != "" {
		details["anthropic_error_type"] = strings.TrimSpace(errType)
	}
	if msg, ok := errObj["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg), details
	}
	return fallback, details
}

func anthropicErrorObject(root map[string]any) bool {
	if root == nil {
		return false
	}
	for _, key := range []string{"type", "message"} {
		if _, ok := root[key]; ok {
			return true
		}
	}
	return false
}

func parseAnthropicUsage(body []byte) *Usage {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}
	usageObj, ok := root["usage"].(map[string]any)
	if !ok {
		return nil
	}
	input, hasInput := usageObj["input_tokens"].(float64)
	output, hasOutput := usageObj["output_tokens"].(float64)
	cacheCreate, hasCacheCreate := usageObj["cache_creation_input_tokens"].(float64)
	cacheRead, hasCacheRead := usageObj["cache_read_input_tokens"].(float64)

	if !hasInput && !hasOutput && !hasCacheCreate && !hasCacheRead {
		return nil
	}
	u := &Usage{}
	if hasInput {
		iv := int(input)
		u.InputTokens = &iv
	}
	if hasOutput {
		ov := int(output)
		u.OutputTokens = &ov
	}
	if hasCacheCreate {
		cv := int(cacheCreate)
		u.CacheCreationInputTokens = &cv
	}
	if hasCacheRead {
		rv := int(cacheRead)
		u.CacheReadInputTokens = &rv
	}
	return u
}

type anthropicStreamEvent struct {
	Type         string                  `json:"type"`
	Index        *int                    `json:"index"`
	ContentBlock *anthropicStreamBlock   `json:"content_block"`
	Delta        *anthropicStreamDelta   `json:"delta"`
	Message      *anthropicStreamMessage `json:"message"`
	Usage        map[string]any          `json:"usage"`
	Error        *anthropicStreamError   `json:"error"`
}

type anthropicStreamMessage struct {
	Usage map[string]any `json:"usage"`
}

type anthropicStreamBlock struct {
	Type     string         `json:"type"`
	Text     string         `json:"text"`
	Thinking string         `json:"thinking"`
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Input    map[string]any `json:"input"`
}

type anthropicStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Thinking    string `json:"thinking"`
	PartialJSON string `json:"partial_json"`
	Signature   string `json:"signature"`
	StopReason  string `json:"stop_reason"`
}

type anthropicToolUseBuffer struct {
	ID   string
	Name string
	JSON strings.Builder
}

type anthropicAssistantBlock struct {
	Type      string
	Text      strings.Builder
	Signature string
	DeltaSeen bool
}

type anthropicStreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
