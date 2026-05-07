package data

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ThreadContextSupersessionEdgeRecord struct {
	ID                      uuid.UUID
	AccountID               uuid.UUID
	ThreadID                uuid.UUID
	ReplacementID           uuid.UUID
	SupersededReplacementID *uuid.UUID
	SupersededChunkID       *uuid.UUID
	CreatedAt               time.Time
}

type ThreadContextSupersessionEdgeInsertInput struct {
	AccountID               uuid.UUID
	ThreadID                uuid.UUID
	ReplacementID           uuid.UUID
	SupersededReplacementID *uuid.UUID
	SupersededChunkID       *uuid.UUID
}

type ThreadContextSupersessionEdgesRepository struct{}
type ReplacementSupersessionEdgeRecord = ThreadContextSupersessionEdgeRecord
type ReplacementSupersessionEdgeInsertInput = ThreadContextSupersessionEdgeInsertInput
type ReplacementSupersessionEdgesRepository = ThreadContextSupersessionEdgesRepository

func (ThreadContextSupersessionEdgesRepository) Insert(
	ctx context.Context,
	tx pgx.Tx,
	input ThreadContextSupersessionEdgeInsertInput,
) (*ThreadContextSupersessionEdgeRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if input.AccountID == uuid.Nil || input.ThreadID == uuid.Nil || input.ReplacementID == uuid.Nil {
		return nil, fmt.Errorf("account_id, thread_id and replacement_id must not be empty")
	}
	hasReplacement := input.SupersededReplacementID != nil && *input.SupersededReplacementID != uuid.Nil
	hasChunk := input.SupersededChunkID != nil && *input.SupersededChunkID != uuid.Nil
	if hasReplacement == hasChunk {
		return nil, fmt.Errorf("exactly one superseded target must be provided")
	}
	if err := ensureReplacementOwnership(ctx, tx, input.AccountID, input.ThreadID, input.ReplacementID); err != nil {
		return nil, err
	}
	if hasReplacement {
		if err := ensureReplacementOwnership(ctx, tx, input.AccountID, input.ThreadID, *input.SupersededReplacementID); err != nil {
			return nil, err
		}
	}
	if hasChunk {
		if err := ensureChunkOwnership(ctx, tx, input.AccountID, input.ThreadID, *input.SupersededChunkID); err != nil {
			return nil, err
		}
	}
	if err := validateReplacementOwnership(ctx, tx, input.AccountID, input.ThreadID, input.ReplacementID); err != nil {
		return nil, err
	}
	if hasReplacement {
		if err := validateReplacementOwnership(ctx, tx, input.AccountID, input.ThreadID, *input.SupersededReplacementID); err != nil {
			return nil, err
		}
	}
	if hasChunk {
		if err := validateSupersededChunkOwnership(ctx, tx, input.AccountID, input.ThreadID, *input.SupersededChunkID); err != nil {
			return nil, err
		}
	}

	var item ThreadContextSupersessionEdgeRecord
	err := tx.QueryRow(
		ctx,
		`INSERT INTO replacement_supersession_edges (
			account_id, thread_id, replacement_id, superseded_replacement_id, superseded_chunk_id
		) VALUES (
			$1, $2, $3, $4, $5
		)
		RETURNING id, account_id, thread_id, replacement_id, superseded_replacement_id, superseded_chunk_id, created_at`,
		input.AccountID,
		input.ThreadID,
		input.ReplacementID,
		input.SupersededReplacementID,
		input.SupersededChunkID,
	).Scan(
		&item.ID,
		&item.AccountID,
		&item.ThreadID,
		&item.ReplacementID,
		&item.SupersededReplacementID,
		&item.SupersededChunkID,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (ThreadContextSupersessionEdgesRepository) ListByReplacementID(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	replacementID uuid.UUID,
) ([]ThreadContextSupersessionEdgeRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil || replacementID == uuid.Nil {
		return nil, fmt.Errorf("account_id, thread_id and replacement_id must not be empty")
	}

	rows, err := tx.Query(
		ctx,
		`SELECT id, account_id, thread_id, replacement_id, superseded_replacement_id, superseded_chunk_id, created_at
		   FROM replacement_supersession_edges
		  WHERE account_id = $1
		    AND thread_id = $2
		    AND replacement_id = $3
		  ORDER BY created_at ASC, id ASC`,
		accountID,
		threadID,
		replacementID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ThreadContextSupersessionEdgeRecord, 0)
	for rows.Next() {
		var item ThreadContextSupersessionEdgeRecord
		if err := rows.Scan(
			&item.ID,
			&item.AccountID,
			&item.ThreadID,
			&item.ReplacementID,
			&item.SupersededReplacementID,
			&item.SupersededChunkID,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (ThreadContextSupersessionEdgesRepository) ListByThread(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
) ([]ThreadContextSupersessionEdgeRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return nil, fmt.Errorf("account_id and thread_id must not be empty")
	}

	rows, err := tx.Query(
		ctx,
		`SELECT id, account_id, thread_id, replacement_id, superseded_replacement_id, superseded_chunk_id, created_at
		   FROM replacement_supersession_edges
		  WHERE account_id = $1
		    AND thread_id = $2
		  ORDER BY replacement_id ASC, created_at ASC, id ASC`,
		accountID,
		threadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ThreadContextSupersessionEdgeRecord, 0)
	for rows.Next() {
		var item ThreadContextSupersessionEdgeRecord
		if err := rows.Scan(
			&item.ID,
			&item.AccountID,
			&item.ThreadID,
			&item.ReplacementID,
			&item.SupersededReplacementID,
			&item.SupersededChunkID,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (ThreadContextSupersessionEdgesRepository) DeleteBySupersededChunkIDs(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	chunkIDs []uuid.UUID,
) error {
	if tx == nil {
		return fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return fmt.Errorf("account_id and thread_id must not be empty")
	}
	if len(chunkIDs) == 0 {
		return fmt.Errorf("chunk_ids must not be empty")
	}
	filtered := make([]uuid.UUID, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		if chunkID == uuid.Nil {
			continue
		}
		filtered = append(filtered, chunkID)
	}
	if len(filtered) == 0 {
		return fmt.Errorf("chunk_ids must include at least one non-empty id")
	}
	if _, err := tx.Exec(
		ctx,
		`DELETE FROM replacement_supersession_edges
		  WHERE account_id = $1
		    AND thread_id = $2
		    AND superseded_chunk_id = ANY($3::uuid[])`,
		accountID,
		threadID,
		filtered,
	); err != nil {
		return err
	}
	return nil
}

func ensureReplacementOwnership(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	replacementID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM thread_context_replacements
			 WHERE id = $1
			   AND account_id = $2
			   AND thread_id = $3
		)`,
		replacementID,
		accountID,
		threadID,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("replacement_id does not belong to account/thread")
	}
	return nil
}

func ensureChunkOwnership(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	chunkID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM thread_context_chunks
			 WHERE id = $1
			   AND account_id = $2
			   AND thread_id = $3
		)`,
		chunkID,
		accountID,
		threadID,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("chunk_id does not belong to account/thread")
	}
	return nil
}

func validateReplacementOwnership(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	replacementID uuid.UUID,
) error {
	var replacementAccountID uuid.UUID
	var replacementThreadID uuid.UUID
	err := tx.QueryRow(
		ctx,
		`SELECT account_id, thread_id
		   FROM thread_context_replacements
		  WHERE id = $1`,
		replacementID,
	).Scan(&replacementAccountID, &replacementThreadID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("replacement_id not found")
		}
		return err
	}
	if replacementAccountID != accountID || replacementThreadID != threadID {
		return fmt.Errorf("replacement_id does not belong to the provided account/thread")
	}
	return nil
}

func validateSupersededChunkOwnership(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	chunkID uuid.UUID,
) error {
	var chunkAccountID uuid.UUID
	var chunkThreadID uuid.UUID
	err := tx.QueryRow(
		ctx,
		`SELECT account_id, thread_id
		   FROM thread_context_chunks
		  WHERE id = $1`,
		chunkID,
	).Scan(&chunkAccountID, &chunkThreadID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("superseded_chunk_id not found")
		}
		return err
	}
	if chunkAccountID != accountID || chunkThreadID != threadID {
		return fmt.Errorf("superseded_chunk_id does not belong to the provided account/thread")
	}
	return nil
}
