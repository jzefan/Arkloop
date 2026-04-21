package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"arkloop/services/shared/telegrambot"
	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
)

func SendTelegramStickerByID(
	ctx context.Context,
	db data.QueryDB,
	store interface {
		Get(ctx context.Context, key string) ([]byte, error)
	},
	client *telegrambot.Client,
	token string,
	target ChannelDeliveryTarget,
	accountID uuid.UUID,
	stickerID string,
) ([]string, error) {
	if db == nil || store == nil || client == nil {
		return nil, fmt.Errorf("sticker delivery unavailable")
	}
	sticker, err := data.AccountStickersRepository{}.GetByHash(ctx, db, accountID, strings.TrimSpace(stickerID))
	if err != nil {
		return nil, err
	}
	if sticker == nil {
		return nil, fmt.Errorf("sticker not found: %s", strings.TrimSpace(stickerID))
	}
	blob, err := store.Get(ctx, sticker.StorageKey)
	if err != nil {
		return nil, err
	}
	replyTo := ""
	if target.ReplyTo != nil {
		replyTo = strings.TrimSpace(target.ReplyTo.MessageID)
	}
	threadID := ""
	if target.Conversation.ThreadID != nil {
		threadID = strings.TrimSpace(*target.Conversation.ThreadID)
	}
	sent, err := client.SendStickerBytes(
		ctx,
		token,
		target.Conversation.Target,
		blob,
		filepath.Base(sticker.StorageKey),
		threadID,
		replyTo,
	)
	if err != nil {
		return nil, err
	}
	if sent == nil {
		return nil, nil
	}
	if incErr := (data.AccountStickersRepository{}).IncrementUsage(ctx, asStickerWriteDB(db), accountID, sticker.ContentHash); incErr != nil {
		return nil, incErr
	}
	return []string{strconv.FormatInt(sent.MessageID, 10)}, nil
}

func asStickerWriteDB(db data.QueryDB) data.DB {
	if typed, ok := db.(data.DB); ok {
		return typed
	}
	return nil
}
