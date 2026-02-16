package data

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type MessagesRepository struct{}

func (MessagesRepository) InsertAssistantMessage(
	ctx context.Context,
	tx pgx.Tx,
	orgID uuid.UUID,
	threadID uuid.UUID,
	content string,
) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	_, err := tx.Exec(
		ctx,
		`INSERT INTO messages (
			org_id, thread_id, created_by_user_id, role, content
		) VALUES (
			$1, $2, NULL, $3, $4
		)`,
		orgID,
		threadID,
		"assistant",
		content,
	)
	return err
}

