package accountapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/http/conversationapi"
	"arkloop/services/shared/messagecontent"
	"arkloop/services/shared/objectstore"
	"arkloop/services/shared/telegrambot"

	"github.com/google/uuid"
)

// MessageAttachmentPutStore Telegram 入站媒体写入对象存储所需的最小接口。
type MessageAttachmentPutStore interface {
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}

func shouldIngestTelegramAttachment(att telegramInboundAttachment) bool {
	switch strings.TrimSpace(att.Type) {
	case "image", "sticker":
		return strings.TrimSpace(att.FileID) != ""
	case "document":
		mt := strings.ToLower(strings.TrimSpace(att.MimeType))
		return strings.HasPrefix(mt, "image/") && strings.TrimSpace(att.FileID) != ""
	default:
		return false
	}
}

func telegramAttachmentDeclaredMime(att telegramInboundAttachment, sniffed string) string {
	if m := strings.TrimSpace(att.MimeType); m != "" {
		return m
	}
	return sniffed
}

func defaultFilenameForTelegramAttachment(att telegramInboundAttachment, mime string) string {
	m := strings.ToLower(strings.TrimSpace(mime))
	switch strings.TrimSpace(att.Type) {
	case "sticker":
		if strings.Contains(m, "webp") {
			return "sticker.webp"
		}
		if strings.Contains(m, "png") {
			return "sticker.png"
		}
		return "sticker.webp"
	case "image":
		switch {
		case strings.Contains(m, "png"):
			return "image.png"
		case strings.Contains(m, "gif"):
			return "image.gif"
		case strings.Contains(m, "webp"):
			return "image.webp"
		default:
			return "image.jpg"
		}
	default:
		if n := strings.TrimSpace(att.FileName); n != "" {
			return conversationapi.SanitizeAttachmentFilename(n)
		}
		return "image.jpg"
	}
}

func ingestTelegramMediaAttachments(
	ctx context.Context,
	client *telegrambot.Client,
	store MessageAttachmentPutStore,
	token string,
	accountID, threadID, userID uuid.UUID,
	items []telegramInboundAttachment,
) (ingested []messagecontent.Part, remaining []telegramInboundAttachment, err error) {
	for _, att := range items {
		if !shouldIngestTelegramAttachment(att) {
			remaining = append(remaining, att)
			continue
		}
		tf, gerr := client.GetFile(ctx, token, att.FileID)
		if gerr != nil {
			return nil, nil, gerr
		}
		data, sniffed, derr := client.DownloadBotFile(ctx, token, tf.FilePath, conversationapi.MaxImageAttachmentBytes)
		if derr != nil {
			return nil, nil, derr
		}
		declared := telegramAttachmentDeclaredMime(att, sniffed)
		displayFilename := strings.TrimSpace(conversationapi.SanitizeAttachmentFilename(att.FileName))
		if displayFilename == "" {
			displayFilename = defaultFilenameForTelegramAttachment(att, declared)
		} else if !strings.Contains(displayFilename, ".") {
			displayFilename += extForImageMIME(declared)
		}
		payload, perr := conversationapi.BuildAttachmentUploadPayload(displayFilename, declared, data)
		if perr != nil {
			return nil, nil, perr
		}
		keySuffix := conversationapi.SanitizeAttachmentKeyName(displayFilename)
		key := fmt.Sprintf("threads/%s/attachments/%s/%s", threadID.String(), uuid.NewString(), keySuffix)
		threadIDText := threadID.String()
		meta := objectstore.ArtifactMetadata(
			conversationapi.MessageAttachmentOwnerKind,
			userID.String(),
			accountID.String(),
			&threadIDText,
		)
		if perr := store.PutObject(ctx, key, payload.Bytes, objectstore.PutOptions{ContentType: payload.MimeType, Metadata: meta}); perr != nil {
			return nil, nil, perr
		}
		ref := &messagecontent.AttachmentRef{
			Key:      key,
			Filename: displayFilename,
			MimeType: payload.MimeType,
			Size:     int64(len(payload.Bytes)),
		}
		switch payload.Kind {
		case messagecontent.PartTypeImage:
			ingested = append(ingested, messagecontent.Part{Type: messagecontent.PartTypeImage, Attachment: ref})
		case messagecontent.PartTypeFile:
			ingested = append(ingested, messagecontent.Part{
				Type:          messagecontent.PartTypeFile,
				Attachment:    ref,
				ExtractedText: payload.ExtractedText,
			})
		default:
			return nil, nil, fmt.Errorf("telegram inbound: unexpected attachment kind %q", payload.Kind)
		}
	}
	return ingested, remaining, nil
}

func extForImageMIME(mime string) string {
	m := strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	switch m {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

func buildTelegramStructuredMessageWithMedia(
	ctx context.Context,
	client *telegrambot.Client,
	store MessageAttachmentPutStore,
	token string,
	accountID, threadID, userID uuid.UUID,
	identity data.ChannelIdentity,
	incoming telegramIncomingMessage,
) (string, json.RawMessage, json.RawMessage, error) {
	displayName := telegramInboundDisplayName(identity, incoming)
	if store == nil || client == nil || strings.TrimSpace(token) == "" {
		return buildTelegramStructuredMessage(identity, incoming)
	}

	mediaParts, remaining, err := ingestTelegramMediaAttachments(ctx, client, store, token, accountID, threadID, userID, incoming.MediaAttachments)
	if err != nil {
		return "", nil, nil, err
	}

	userBody := strings.TrimSpace(incoming.Text)
	attachBlock := renderTelegramAttachmentBlock(remaining)
	if attachBlock != "" {
		if userBody != "" {
			userBody += "\n\n" + attachBlock
		} else {
			userBody = attachBlock
		}
	}
	userBody = strings.TrimSpace(userBody)
	if userBody == "" && len(mediaParts) == 0 {
		return "", nil, nil, fmt.Errorf("telegram inbound message content is empty")
	}

	envelope := buildTelegramEnvelopeText(identity.ID, incoming, displayName, userBody)
	parts := []messagecontent.Part{{Type: messagecontent.PartTypeText, Text: envelope}}
	parts = append(parts, mediaParts...)

	content, err := messagecontent.Normalize(parts)
	if err != nil {
		return "", nil, nil, err
	}

	_, projection, raw, err := conversationapi.FinalizeMessageContent(content)
	if err != nil {
		return "", nil, nil, err
	}
	metadataJSON, err := telegramInboundMetadataJSON(identity, incoming, displayName)
	if err != nil {
		return "", nil, nil, err
	}
	return projection, raw, metadataJSON, nil
}
