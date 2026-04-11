package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type RunPipelineEventsRepository struct {
	db DB
}

type RunPipelineEventRow struct {
	ID         int64
	RunID      uuid.UUID
	AccountID  uuid.UUID
	Middleware string
	EventName  string
	Seq        int
	FieldsJSON map[string]any
	CreatedAt  time.Time
}

func NewRunPipelineEventsRepository(db DB) *RunPipelineEventsRepository {
	if db == nil {
		return nil
	}
	return &RunPipelineEventsRepository{db: db}
}

func (r *RunPipelineEventsRepository) ListByRunID(ctx context.Context, runID uuid.UUID, limit int) ([]RunPipelineEventRow, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 1000
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, run_id, account_id, middleware, event_name, seq, fields_json, CAST(created_at AS TEXT)
		   FROM run_pipeline_events
		  WHERE run_id = $1
		  ORDER BY seq ASC, created_at ASC
		  LIMIT $2`,
		runID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RunPipelineEventRow, 0, limit)
	for rows.Next() {
		var item RunPipelineEventRow
		var createdAtText string
		var raw any
		if err := rows.Scan(
			&item.ID,
			&item.RunID,
			&item.AccountID,
			&item.Middleware,
			&item.EventName,
			&item.Seq,
			&raw,
			&createdAtText,
		); err != nil {
			return nil, err
		}
		item.CreatedAt, err = parseFlexibleTimestamp(createdAtText)
		if err != nil {
			return nil, err
		}
		if payload := jsonPayloadBytes(raw); len(payload) > 0 {
			if err := json.Unmarshal(payload, &item.FieldsJSON); err != nil {
				return nil, err
			}
		}
		if item.FieldsJSON == nil {
			item.FieldsJSON = map[string]any{}
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func jsonPayloadBytes(raw any) []byte {
	switch value := raw.(type) {
	case nil:
		return nil
	case []byte:
		return value
	case string:
		return []byte(value)
	default:
		return []byte(fmt.Sprint(value))
	}
}

func parseFlexibleTimestamp(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse pipeline event timestamp: %q", value)
}
