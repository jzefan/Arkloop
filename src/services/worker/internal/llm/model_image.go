package llm

import (
	"encoding/base64"
	"fmt"
	"strings"

	"arkloop/services/worker/internal/imageutil"
)

type preparedModelInputImage struct {
	MimeType         string
	Data             []byte
	Base64           string
	DataURL          string
	Base64ImageBytes int
	Err              error
}

func PrepareRequestModelInputImages(req *Request) {
	if req == nil || req.modelInputImagesPrepared {
		return
	}

	var messages []Message
	for msgIndex, msg := range req.Messages {
		var parts []ContentPart
		for partIndex, part := range msg.Content {
			if part.Kind() != "image" {
				continue
			}
			if messages == nil {
				messages = make([]Message, len(req.Messages))
				copy(messages, req.Messages)
			}
			if parts == nil {
				parts = make([]ContentPart, len(msg.Content))
				copy(parts, msg.Content)
				messages[msgIndex].Content = parts
			}
			prepared := prepareModelInputImage(part)
			parts[partIndex].modelInputImage = &prepared
		}
	}
	if messages != nil {
		req.Messages = messages
	}
	req.modelInputImagesPrepared = true
}

func modelInputImage(part ContentPart) (string, []byte, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return "", nil, prepared.Err
	}
	return prepared.MimeType, prepared.Data, nil
}

func modelInputImagePayload(part ContentPart) preparedModelInputImage {
	if part.modelInputImage != nil {
		return *part.modelInputImage
	}
	return prepareModelInputImage(part)
}

func prepareModelInputImage(part ContentPart) preparedModelInputImage {
	if part.Attachment == nil {
		return preparedModelInputImage{Err: fmt.Errorf("image attachment is required")}
	}
	if len(part.Data) == 0 {
		return preparedModelInputImage{Err: fmt.Errorf("image attachment data is required")}
	}

	mimeType := strings.TrimSpace(part.Attachment.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	data := part.Data
	if key := strings.TrimSpace(part.Attachment.Key); key != "" {
		data, mimeType = imageutil.PrepareModelInputImage(data, mimeType, key)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return preparedModelInputImage{
		MimeType:         mimeType,
		Data:             data,
		Base64:           encoded,
		DataURL:          "data:" + mimeType + ";base64," + encoded,
		Base64ImageBytes: len(encoded),
	}
}

func modelInputImageDataURL(part ContentPart) (string, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return "", prepared.Err
	}
	return prepared.DataURL, nil
}

func modelInputImageBase64(part ContentPart) (string, string, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return "", "", prepared.Err
	}
	return prepared.MimeType, prepared.Base64, nil
}

func modelInputImageBase64Size(part ContentPart) (int, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return 0, prepared.Err
	}
	return prepared.Base64ImageBytes, nil
}
