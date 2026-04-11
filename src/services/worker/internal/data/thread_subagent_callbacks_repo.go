package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ThreadSubAgentCallbackRecord struct {
	ID              uuid.UUID
	AccountID       uuid.UUID
	ThreadID        uuid.UUID
	SubAgentID      uuid.UUID
	SourceRunID     uuid.UUID
	Status          string
	PayloadJSON     map[string]any
	CreatedAt       time.Time
	ConsumedAt      *time.Time
	ConsumedByRunID *uuid.UUID
}

type ThreadSubAgentCallbackCreateParams struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	ThreadID    uuid.UUID
	SubAgentID  uuid.UUID
	SourceRunID uuid.UUID
	Status      string
	PayloadJSON map[string]any
}

type ThreadSubAgentCallbacksRepository struct{}

func (ThreadSubAgentCallbacksRepository) Insert(ctx context.Context, tx pgx.Tx, params ThreadSubAgentCallbackCreateParams) (ThreadSubAgentCallbackRecord, error) {
	if tx == nil {
		return ThreadSubAgentCallbackRecord{}, fmt.Errorf("tx must not be nil")
	}
	if params.ID == uuid.Nil {
		params.ID = uuid.New()
	}
	if params.AccountID == uuid.Nil || params.ThreadID == uuid.Nil || params.SubAgentID == uuid.Nil || params.SourceRunID == uuid.Nil {
		return ThreadSubAgentCallbackRecord{}, fmt.Errorf("callback identity fields must not be empty")
	}
	encoded, err := json.Marshal(mapOrEmpty(params.PayloadJSON))
	if err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	var record ThreadSubAgentCallbackRecord
	err = tx.QueryRow(ctx,
		`INSERT INTO thread_subagent_callbacks (
			id, account_id, thread_id, sub_agent_id, source_run_id, status, payload_json
		 ) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		 )
		 RETURNING id, account_id, thread_id, sub_agent_id, source_run_id, status, payload_json, created_at, consumed_at, consumed_by_run_id`,
		params.ID,
		params.AccountID,
		params.ThreadID,
		params.SubAgentID,
		params.SourceRunID,
		params.Status,
		string(encoded),
	).Scan(
		&record.ID,
		&record.AccountID,
		&record.ThreadID,
		&record.SubAgentID,
		&record.SourceRunID,
		&record.Status,
		&encoded,
		&record.CreatedAt,
		&record.ConsumedAt,
		&record.ConsumedByRunID,
	)
	if err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	if err := json.Unmarshal(encoded, &record.PayloadJSON); err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	return record, nil
}

func (ThreadSubAgentCallbacksRepository) ListPendingByThread(ctx context.Context, db QueryDB, threadID uuid.UUID) ([]ThreadSubAgentCallbackRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	if threadID == uuid.Nil {
		return nil, fmt.Errorf("thread_id must not be empty")
	}
	rows, err := db.Query(ctx,
		`SELECT id, account_id, thread_id, sub_agent_id, source_run_id, status, payload_json, created_at, consumed_at, consumed_by_run_id
		   FROM thread_subagent_callbacks
		  WHERE thread_id = $1
		    AND consumed_at IS NULL
		  ORDER BY created_at ASC, id ASC`,
		threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ThreadSubAgentCallbackRecord, 0)
	for rows.Next() {
		record, err := scanThreadSubAgentCallback(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (ThreadSubAgentCallbacksRepository) GetPendingByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*ThreadSubAgentCallbackRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if id == uuid.Nil {
		return nil, fmt.Errorf("id must not be empty")
	}
	row := tx.QueryRow(ctx,
		`SELECT id, account_id, thread_id, sub_agent_id, source_run_id, status, payload_json, created_at, consumed_at, consumed_by_run_id
		   FROM thread_subagent_callbacks
		  WHERE id = $1
		    AND consumed_at IS NULL
		  LIMIT 1`,
		id,
	)
	record, err := scanThreadSubAgentCallbackRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (ThreadSubAgentCallbacksRepository) MarkConsumed(ctx context.Context, tx pgx.Tx, id uuid.UUID, consumedByRunID uuid.UUID) error {
	if tx == nil {
		return fmt.Errorf("tx must not be nil")
	}
	if id == uuid.Nil || consumedByRunID == uuid.Nil {
		return fmt.Errorf("callback id and consumed_by_run_id must not be empty")
	}
	_, err := tx.Exec(ctx,
		`UPDATE thread_subagent_callbacks
		    SET consumed_at = now(),
		        consumed_by_run_id = $2
		  WHERE id = $1
		    AND consumed_at IS NULL`,
		id,
		consumedByRunID,
	)
	return err
}

func scanThreadSubAgentCallback(rows pgx.Rows) (ThreadSubAgentCallbackRecord, error) {
	var record ThreadSubAgentCallbackRecord
	var raw []byte
	if err := rows.Scan(
		&record.ID,
		&record.AccountID,
		&record.ThreadID,
		&record.SubAgentID,
		&record.SourceRunID,
		&record.Status,
		&raw,
		&record.CreatedAt,
		&record.ConsumedAt,
		&record.ConsumedByRunID,
	); err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	if err := json.Unmarshal(raw, &record.PayloadJSON); err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	return record, nil
}

func scanThreadSubAgentCallbackRow(row pgx.Row) (ThreadSubAgentCallbackRecord, error) {
	var record ThreadSubAgentCallbackRecord
	var raw []byte
	if err := row.Scan(
		&record.ID,
		&record.AccountID,
		&record.ThreadID,
		&record.SubAgentID,
		&record.SourceRunID,
		&record.Status,
		&raw,
		&record.CreatedAt,
		&record.ConsumedAt,
		&record.ConsumedByRunID,
	); err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	if err := json.Unmarshal(raw, &record.PayloadJSON); err != nil {
		return ThreadSubAgentCallbackRecord{}, err
	}
	return record, nil
}
