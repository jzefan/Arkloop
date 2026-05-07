package llm

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"arkloop/services/shared/messagecontent"
)

const testBannerHeight = 100

func makeVisionTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fill := color.RGBA{R: 220, G: 30, B: 30, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func decodeModelImage(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	return img
}

func decodeDataURLPayload(t *testing.T, dataURL string) image.Image {
	t.Helper()
	idx := strings.Index(dataURL, ",")
	if idx < 0 {
		t.Fatalf("invalid data url: %q", dataURL)
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[idx+1:])
	if err != nil {
		t.Fatalf("decode data url: %v", err)
	}
	return decodeModelImage(t, raw)
}

func decodeInlineDataPayload(t *testing.T, raw string) image.Image {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode inline data: %v", err)
	}
	return decodeModelImage(t, data)
}

func TestPartDataURLAnnotatesAttachmentKeyImage(t *testing.T) {
	part := ContentPart{
		Type: "image",
		Attachment: &messagecontent.AttachmentRef{
			Key:      "attachments/acc/thread/image.jpg",
			Filename: "image.jpg",
			MimeType: "image/png",
		},
		Data: makeVisionTestPNG(t, 320, 180),
	}

	dataURL, err := partDataURL(part)
	if err != nil {
		t.Fatalf("partDataURL failed: %v", err)
	}
	if !strings.HasPrefix(dataURL, "data:image/jpeg;base64,") {
		t.Fatalf("unexpected data url prefix: %q", dataURL[:24])
	}

	img := decodeDataURLPayload(t, dataURL)
	if got := img.Bounds().Dy(); got != 180+testBannerHeight {
		t.Fatalf("unexpected annotated height: %d", got)
	}
}

func TestPrepareRequestModelInputImagesReusesPreparedImage(t *testing.T) {
	req := Request{
		Model: "gpt",
		Messages: []Message{{
			Role: "user",
			Content: []ContentPart{{
				Type: "image",
				Attachment: &messagecontent.AttachmentRef{
					Key:      "attachments/acc/thread/image.jpg",
					Filename: "image.jpg",
					MimeType: "image/png",
				},
				Data: makeVisionTestPNG(t, 220, 130),
			}},
		}},
	}

	PrepareRequestModelInputImages(&req)
	part := req.Messages[0].Content[0]
	if part.modelInputImage == nil {
		t.Fatal("expected prepared image")
	}
	prepared := part.modelInputImage
	if !strings.HasPrefix(prepared.DataURL, "data:image/jpeg;base64,") {
		t.Fatalf("unexpected data url prefix: %q", prepared.DataURL[:24])
	}

	req.Messages[0].Content[0].Data = nil
	PrepareRequestModelInputImages(&req)
	if req.Messages[0].Content[0].modelInputImage != prepared {
		t.Fatal("prepared image should remain scoped to the request")
	}

	stats := ComputeRequestStats(req)
	if stats.ImagePartCount != 1 {
		t.Fatalf("unexpected image count: %d", stats.ImagePartCount)
	}
	if stats.Base64ImageBytes != prepared.Base64ImageBytes {
		t.Fatalf("unexpected base64 bytes: got %d want %d", stats.Base64ImageBytes, prepared.Base64ImageBytes)
	}

	chatMessages, err := toOpenAIChatMessages(req.Messages)
	if err != nil {
		t.Fatalf("toOpenAIChatMessages failed: %v", err)
	}
	chatContent := chatMessages[0]["content"].([]map[string]any)
	chatImage := chatContent[1]["image_url"].(map[string]any)
	if got := chatImage["url"]; got != prepared.DataURL {
		t.Fatalf("chat payload did not reuse prepared data url")
	}

	responsesInput, err := toOpenAIResponsesInput(req.Messages)
	if err != nil {
		t.Fatalf("toOpenAIResponsesInput failed: %v", err)
	}
	responsesContent := responsesInput[0]["content"].([]map[string]any)
	if got := responsesContent[1]["image_url"]; got != prepared.DataURL {
		t.Fatalf("responses payload did not reuse prepared data url")
	}
}

func TestPrepareRequestModelInputImagesPreparesAllImageParts(t *testing.T) {
	req := Request{
		Model: "gpt",
		Messages: []Message{
			{
				Role: "user",
				Content: []ContentPart{
					{Type: messagecontent.PartTypeText, Text: "before"},
					{
						Type: messagecontent.PartTypeImage,
						Attachment: &messagecontent.AttachmentRef{
							Key:      "attachments/image-a.png",
							MimeType: "image/png",
						},
						Data: []byte("image-a"),
					},
				},
			},
			{
				Role: "user",
				Content: []ContentPart{
					{
						Type: messagecontent.PartTypeImage,
						Attachment: &messagecontent.AttachmentRef{
							Key:      "attachments/image-b.png",
							MimeType: "image/png",
						},
						Data: []byte("image-b"),
					},
					{
						Type: messagecontent.PartTypeImage,
						Attachment: &messagecontent.AttachmentRef{
							Key:      "attachments/image-c.png",
							MimeType: "image/png",
						},
						Data: []byte("image-c"),
					},
				},
			},
		},
	}

	PrepareRequestModelInputImages(&req)
	if !req.modelInputImagesPrepared {
		t.Fatal("expected request to be marked prepared")
	}
	if req.Messages[0].Content[0].modelInputImage != nil {
		t.Fatal("text part should not be prepared as image")
	}

	for _, item := range []struct {
		part ContentPart
		want string
	}{
		{part: req.Messages[0].Content[1], want: "image-a"},
		{part: req.Messages[1].Content[0], want: "image-b"},
		{part: req.Messages[1].Content[1], want: "image-c"},
	} {
		if item.part.modelInputImage == nil {
			t.Fatal("expected image part to be prepared")
		}
		if got := string(item.part.modelInputImage.Data); got != item.want {
			t.Fatalf("unexpected prepared data: got=%q want=%q", got, item.want)
		}
	}
}

func TestPrepareRequestModelInputImagesReusesProcessCache(t *testing.T) {
	modelInputImageCache.Lock()
	modelInputImageCache.order = nil
	modelInputImageCache.items = map[string]preparedModelInputImage{}
	modelInputImageCache.Unlock()

	part := ContentPart{
		Type: messagecontent.PartTypeImage,
		Attachment: &messagecontent.AttachmentRef{
			Key:      "attachments/cache-image.png",
			MimeType: "image/png",
		},
		Data: []byte("cache-image"),
	}
	req := Request{Model: "gpt", Messages: []Message{{Role: "user", Content: []ContentPart{part}}}}
	PrepareRequestModelInputImages(&req)
	if req.Messages[0].Content[0].modelInputImage == nil {
		t.Fatal("expected first request to prepare image")
	}

	cacheKey, ok := modelInputImageCacheKey(part)
	if !ok {
		t.Fatal("expected cache key")
	}
	modelInputImageCache.Lock()
	cached := modelInputImageCache.items[cacheKey]
	cached.Data = []byte("cached-image")
	modelInputImageCache.items[cacheKey] = cached
	modelInputImageCache.Unlock()

	next := Request{Model: "gpt", Messages: []Message{{Role: "user", Content: []ContentPart{part}}}}
	PrepareRequestModelInputImages(&next)
	prepared := next.Messages[0].Content[0].modelInputImage
	if prepared == nil {
		t.Fatal("expected cached request to prepare image")
	}
	if got := string(prepared.Data); got != "cached-image" {
		t.Fatalf("expected cached prepared data, got %q", got)
	}
}

func TestToAnthropicMessagesAnnotatesUserImage(t *testing.T) {
	_, messages, err := toAnthropicMessages([]Message{
		{
			Role: "user",
			Content: []ContentPart{
				{Type: "text", Text: "看看这个"},
				{
					Type: "image",
					Attachment: &messagecontent.AttachmentRef{
						Key:      "attachments/acc/thread/image.jpg",
						Filename: "image.jpg",
						MimeType: "image/png",
					},
					Data: makeVisionTestPNG(t, 300, 160),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("toAnthropicMessages failed: %v", err)
	}

	blocks := messages[0]["content"].([]map[string]any)
	if len(blocks) != 3 {
		t.Fatalf("unexpected block count: %#v", blocks)
	}
	source := blocks[2]["source"].(map[string]any)
	if source["media_type"] != "image/jpeg" {
		t.Fatalf("unexpected media type: %#v", source["media_type"])
	}
	img := decodeInlineDataPayload(t, source["data"].(string))
	if got := img.Bounds().Dy(); got != 160+testBannerHeight {
		t.Fatalf("unexpected annotated height: %d", got)
	}
}

func TestToAnthropicMessagesAnnotatesToolResultImage(t *testing.T) {
	_, messages, err := toAnthropicMessages([]Message{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ToolCallID:    "call_1",
				ToolName:      "read",
				ArgumentsJSON: map[string]any{"source": map[string]any{"kind": "message_attachment"}},
			}},
		},
		{
			Role: "tool",
			Content: []ContentPart{
				{Type: "text", Text: `{"tool_call_id":"call_1","tool_name":"read","result":{"ok":true}}`},
				{
					Type: "image",
					Attachment: &messagecontent.AttachmentRef{
						Key:      "attachments/acc/thread/image.jpg",
						Filename: "image.jpg",
						MimeType: "image/png",
					},
					Data: makeVisionTestPNG(t, 240, 140),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("toAnthropicMessages failed: %v", err)
	}

	wrapper := messages[1]["content"].([]map[string]any)[0]
	content := wrapper["content"].([]map[string]any)
	source := content[1]["source"].(map[string]any)
	img := decodeInlineDataPayload(t, source["data"].(string))
	if got := img.Bounds().Dy(); got != 140+testBannerHeight {
		t.Fatalf("unexpected annotated height: %d", got)
	}
}

func TestGeminiUserPartsAnnotatesAttachmentKeyImage(t *testing.T) {
	parts, err := geminiUserParts([]ContentPart{{
		Type: "image",
		Attachment: &messagecontent.AttachmentRef{
			Key:      "attachments/acc/thread/image.jpg",
			Filename: "image.jpg",
			MimeType: "image/png",
		},
		Data: makeVisionTestPNG(t, 260, 150),
	}})
	if err != nil {
		t.Fatalf("geminiUserParts failed: %v", err)
	}

	inline := parts[0]["inlineData"].(map[string]any)
	if inline["mimeType"] != "image/jpeg" {
		t.Fatalf("unexpected mime type: %#v", inline["mimeType"])
	}
	img := decodeInlineDataPayload(t, inline["data"].(string))
	if got := img.Bounds().Dy(); got != 150+testBannerHeight {
		t.Fatalf("unexpected annotated height: %d", got)
	}
}

func TestToGeminiContentsAnnotatesToolImage(t *testing.T) {
	_, contents, err := toGeminiContents([]Message{
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ToolCallID:    "call_1",
				ToolName:      "read",
				ArgumentsJSON: map[string]any{"source": map[string]any{"kind": "message_attachment"}},
			}},
		},
		{
			Role: "tool",
			Content: []ContentPart{
				{Type: "text", Text: `{"tool_call_id":"call_1","tool_name":"read","result":{"ok":true}}`},
				{
					Type: "image",
					Attachment: &messagecontent.AttachmentRef{
						Key:      "attachments/acc/thread/image.jpg",
						Filename: "image.jpg",
						MimeType: "image/png",
					},
					Data: makeVisionTestPNG(t, 200, 120),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("toGeminiContents failed: %v", err)
	}

	toolParts := contents[1]["parts"].([]map[string]any)
	inline := toolParts[1]["inlineData"].(map[string]any)
	img := decodeInlineDataPayload(t, inline["data"].(string))
	if got := img.Bounds().Dy(); got != 120+testBannerHeight {
		t.Fatalf("unexpected annotated height: %d", got)
	}
}
