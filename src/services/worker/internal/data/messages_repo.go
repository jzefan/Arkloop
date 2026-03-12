package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type MessagesRepository struct{
	Dialect database.DialectHelper
}

func (r MessagesRepository) dialect() database.DialectHelper {
	if r.Dialect != nil {
		return r.Dialect
	}
	return database.PostgresDialect{}
}

type ThreadMessage struct {
	Role        string
	Content     string
	ContentJSON json.RawMessage
}

type ConversationSearchHit struct {
	ThreadID  uuid.UUID
	Role      string
	Content   string
	CreatedAt time.Time
}

func (r MessagesRepository) InsertAssistantMessage(
	ctx context.Context,
	tx database.Tx,
	orgID uuid.UUID,
	threadID uuid.UUID,
	runID uuid.UUID,
	content string,
) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	metadataJSON := map[string]any{}
	if runID != uuid.Nil {
		metadataJSON["run_id"] = runID.String()
	}
	metadataRaw, err := json.Marshal(metadataJSON)
	if err != nil {
		return fmt.Errorf("marshal metadata_json: %w", err)
	}
	_, err = tx.Exec(
		ctx,
		fmt.Sprintf(`INSERT INTO messages (
			org_id, thread_id, created_by_user_id, role, content, metadata_json
		) VALUES (
			$1, $2, NULL, $3, $4, %s
		)`, r.dialect().JSONCast("$5")),
		orgID,
		threadID,
		"assistant",
		content,
		string(metadataRaw),
	)
	return err
}

func (MessagesRepository) ListByThread(
	ctx context.Context,
	tx database.Tx,
	orgID uuid.UUID,
	threadID uuid.UUID,
	limit int,
) ([]ThreadMessage, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := tx.Query(
		ctx,
		`SELECT role, content, content_json
		 FROM messages
		 WHERE org_id = $1
		   AND thread_id = $2
		   AND hidden = FALSE
		   AND deleted_at IS NULL
		 ORDER BY created_at ASC
		 LIMIT $3`,
		orgID,
		threadID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ThreadMessage{}
	for rows.Next() {
		var item ThreadMessage
		if err := rows.Scan(&item.Role, &item.Content, &item.ContentJSON); err != nil {
			return nil, err
		}
		item.Role = strings.TrimSpace(item.Role)
		item.Content = strings.TrimSpace(item.Content)
		if item.Role == "" {
			continue
		}
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (r MessagesRepository) SearchVisibleByOwner(
	ctx context.Context,
	pool database.DB,
	orgID uuid.UUID,
	ownerUserID uuid.UUID,
	query string,
	limit int,
) ([]ConversationSearchHit, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool must not be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if limit <= 0 {
		limit = 10
	}

	like := "%" + escapeILikePattern(trimmedQuery) + "%"
	rows, err := pool.Query(
		ctx,
		fmt.Sprintf(`SELECT m.thread_id, m.role, m.content, m.created_at
		 FROM messages m
		 JOIN threads t ON t.id = m.thread_id
		 WHERE m.org_id = $1
		   AND t.org_id = $1
		   AND t.created_by_user_id = $2
		   AND t.deleted_at IS NULL
		   AND t.is_private = FALSE
		   AND m.deleted_at IS NULL
		   AND m.hidden = FALSE
		   AND m.content %s $3 ESCAPE '!'
		 ORDER BY m.created_at DESC, m.id DESC
		 LIMIT $4`, r.dialect().ILike()),
		orgID, ownerUserID, like, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hits := make([]ConversationSearchHit, 0, limit)
	for rows.Next() {
		var item ConversationSearchHit
		if err := rows.Scan(&item.ThreadID, &item.Role, &item.Content, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Role = strings.TrimSpace(item.Role)
		item.Content = strings.TrimSpace(item.Content)
		hits = append(hits, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hits, nil
}

func escapeILikePattern(input string) string {
	replacer := strings.NewReplacer(
		"!", "!!",
		"%", "!%",
		"_", "!_",
	)
	return replacer.Replace(input)
}
