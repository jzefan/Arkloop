package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Notification struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	OrgID       uuid.UUID
	Type        string
	Title       string
	Body        string
	PayloadJSON map[string]any
	ReadAt      *time.Time
	CreatedAt   time.Time
}

type NotificationsRepository struct {
	db Querier
}

func NewNotificationsRepository(db Querier) (*NotificationsRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &NotificationsRepository{db: db}, nil
}

func (r *NotificationsRepository) Create(
	ctx context.Context,
	userID uuid.UUID,
	orgID uuid.UUID,
	notifType string,
	title string,
	body string,
	payloadJSON map[string]any,
) (Notification, error) {
	if userID == uuid.Nil {
		return Notification{}, fmt.Errorf("notifications: user_id must not be empty")
	}
	if orgID == uuid.Nil {
		return Notification{}, fmt.Errorf("notifications: org_id must not be empty")
	}
	if notifType == "" {
		return Notification{}, fmt.Errorf("notifications: type must not be empty")
	}
	if title == "" {
		return Notification{}, fmt.Errorf("notifications: title must not be empty")
	}
	if payloadJSON == nil {
		payloadJSON = map[string]any{}
	}

	var n Notification
	err := r.db.QueryRow(
		ctx,
		`INSERT INTO notifications (user_id, org_id, type, title, body, payload_json)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, user_id, org_id, type, title, body, payload_json, read_at, created_at`,
		userID, orgID, notifType, title, body, payloadJSON,
	).Scan(
		&n.ID, &n.UserID, &n.OrgID, &n.Type, &n.Title,
		&n.Body, &n.PayloadJSON, &n.ReadAt, &n.CreatedAt,
	)
	if err != nil {
		return Notification{}, fmt.Errorf("notifications.Create: %w", err)
	}
	return n, nil
}

func (r *NotificationsRepository) ListUnread(ctx context.Context, userID uuid.UUID) ([]Notification, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("notifications: user_id must not be empty")
	}

	rows, err := r.db.Query(
		ctx,
		`SELECT id, user_id, org_id, type, title, body, payload_json, read_at, created_at
		 FROM notifications
		 WHERE user_id = $1 AND read_at IS NULL
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("notifications.ListUnread: %w", err)
	}
	defer rows.Close()

	var results []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(
			&n.ID, &n.UserID, &n.OrgID, &n.Type, &n.Title,
			&n.Body, &n.PayloadJSON, &n.ReadAt, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("notifications.ListUnread scan: %w", err)
		}
		results = append(results, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notifications.ListUnread rows: %w", err)
	}
	return results, nil
}

func (r *NotificationsRepository) MarkRead(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	if userID == uuid.Nil {
		return fmt.Errorf("notifications: user_id must not be empty")
	}
	if id == uuid.Nil {
		return fmt.Errorf("notifications: id must not be empty")
	}

	tag, err := r.db.Exec(
		ctx,
		`UPDATE notifications
		 SET read_at = now()
		 WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("notifications.MarkRead: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// already read or not found — treat as no-op
		return pgx.ErrNoRows
	}
	return nil
}
