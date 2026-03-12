package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
"arkloop/services/shared/database"
)

type RunEventsRepository struct{
	Dialect database.DialectHelper
}

func (r RunEventsRepository) dialect() database.DialectHelper {
	if r.Dialect != nil {
		return r.Dialect
	}
	return database.PostgresDialect{}
}

func (r RunEventsRepository) GetLatestEventType(
	ctx context.Context,
	tx database.Tx,
	runID uuid.UUID,
	types []string,
) (string, error) {
	if len(types) == 0 {
		return "", nil
	}

	var eventType string
	err := tx.QueryRow(
		ctx,
		fmt.Sprintf(`SELECT type
		 FROM run_events
		 WHERE run_id = $1
		   AND %s
		 ORDER BY seq DESC
		 LIMIT 1`, r.dialect().ArrayAny("type", 2)),
		runID,
		types,
	).Scan(&eventType)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return eventType, nil
}

func (r RunEventsRepository) AppendEvent(
	ctx context.Context,
	tx database.Tx,
	runID uuid.UUID,
	eventType string,
	dataJSON map[string]any,
	toolName *string,
	errorClass *string,
) (int64, error) {
	seq, err := r.allocateSeq(ctx, tx)
	if err != nil {
		return 0, err
	}

	encoded, err := json.Marshal(mapOrEmpty(dataJSON))
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec(
		ctx,
		fmt.Sprintf(`INSERT INTO run_events (
			run_id, seq, type, data_json, tool_name, error_class
		) VALUES (
			$1, $2, $3, %s, $5, $6
		)`, r.dialect().JSONCast("$4")),
		runID,
		seq,
		eventType,
		string(encoded),
		toolName,
		errorClass,
	)
	if err != nil {
		return 0, err
	}

	return seq, nil
}

func (RunEventsRepository) FirstEventData(
	ctx context.Context,
	tx database.Tx,
	runID uuid.UUID,
) (string, map[string]any, error) {
	var (
		eventType string
		rawJSON   []byte
	)
	err := tx.QueryRow(
		ctx,
		`SELECT type, data_json
		 FROM run_events
		 WHERE run_id = $1
		 ORDER BY seq ASC
		 LIMIT 1`,
		runID,
	).Scan(&eventType, &rawJSON)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, err
	}

	if len(rawJSON) == 0 {
		return eventType, nil, nil
	}

	var parsed any
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		return eventType, nil, nil
	}
	obj, ok := parsed.(map[string]any)
	if !ok {
		return eventType, nil, nil
	}
	return eventType, obj, nil
}

func (r RunEventsRepository) allocateSeq(ctx context.Context, tx database.Tx) (int64, error) {
	var seq int64
	err := tx.QueryRow(ctx, fmt.Sprintf("SELECT %s", r.dialect().Sequence("run_events_seq_global"))).Scan(&seq)
	if err != nil {
		return 0, err
	}
	return seq, nil
}

func mapOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}
