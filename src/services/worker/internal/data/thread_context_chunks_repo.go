package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ContextChunkRecord struct {
	ID           uuid.UUID
	AccountID    uuid.UUID
	ThreadID     uuid.UUID
	AtomID       uuid.UUID
	ChunkSeq     int64
	ChunkIndex   int
	ContextSeq   int64
	ChunkKind    string
	Text         string
	PayloadText  string
	PayloadJSON  json.RawMessage
	MetadataJSON json.RawMessage
	CreatedAt    time.Time
}

type ContextChunkInsertInput struct {
	AccountID    uuid.UUID
	ThreadID     uuid.UUID
	AtomID       uuid.UUID
	ChunkSeq     int64
	ChunkIndex   int
	ContextSeq   int64
	ChunkKind    string
	Text         string
	PayloadText  string
	PayloadJSON  json.RawMessage
	MetadataJSON json.RawMessage
}

type ThreadContextChunksRepository struct{}

func (ThreadContextChunksRepository) Insert(
	ctx context.Context,
	tx pgx.Tx,
	input ContextChunkInsertInput,
) (*ContextChunkRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if input.AccountID == uuid.Nil || input.ThreadID == uuid.Nil || input.AtomID == uuid.Nil {
		return nil, fmt.Errorf("account_id, thread_id and atom_id must not be empty")
	}
	if err := validateChunkAtomOwnership(ctx, tx, input.AccountID, input.ThreadID, input.AtomID); err != nil {
		return nil, err
	}
	chunkSeq := input.ChunkSeq
	if chunkSeq <= 0 {
		chunkSeq = int64(input.ChunkIndex + 1)
	}
	if chunkSeq <= 0 || input.ContextSeq <= 0 {
		return nil, fmt.Errorf("chunk_seq and context_seq must be positive")
	}
	chunkKind := strings.TrimSpace(input.ChunkKind)
	if chunkKind == "" {
		chunkKind = "payload"
	}
	payloadJSON := input.PayloadJSON
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}
	metadataJSON := input.MetadataJSON
	if len(metadataJSON) == 0 {
		metadataJSON = json.RawMessage(`{}`)
	}

	payloadText := input.PayloadText
	if strings.TrimSpace(payloadText) == "" {
		payloadText = input.Text
	}

	var item ContextChunkRecord
	err := tx.QueryRow(
		ctx,
		`INSERT INTO thread_context_chunks (
			account_id, thread_id, atom_id, chunk_seq, context_seq, chunk_kind,
			payload_text, payload_json, metadata_json
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb
		)
		ON CONFLICT (thread_id, context_seq) DO UPDATE SET
			atom_id = EXCLUDED.atom_id,
			chunk_seq = EXCLUDED.chunk_seq,
			chunk_kind = EXCLUDED.chunk_kind,
			payload_text = EXCLUDED.payload_text,
			payload_json = EXCLUDED.payload_json,
			metadata_json = EXCLUDED.metadata_json
		RETURNING id, account_id, thread_id, atom_id, chunk_seq, context_seq, chunk_kind,
		          payload_text, payload_json, metadata_json, created_at`,
		input.AccountID,
		input.ThreadID,
		input.AtomID,
		chunkSeq,
		input.ContextSeq,
		chunkKind,
		payloadText,
		string(payloadJSON),
		string(metadataJSON),
	).Scan(
		&item.ID,
		&item.AccountID,
		&item.ThreadID,
		&item.AtomID,
		&item.ChunkSeq,
		&item.ContextSeq,
		&item.ChunkKind,
		&item.PayloadText,
		&item.PayloadJSON,
		&item.MetadataJSON,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.ChunkKind = strings.TrimSpace(item.ChunkKind)
	item.ChunkIndex = int(item.ChunkSeq - 1)
	item.Text = item.PayloadText
	item.PayloadText = strings.TrimSpace(item.PayloadText)
	return &item, nil
}

func (ThreadContextChunksRepository) Upsert(
	ctx context.Context,
	tx pgx.Tx,
	input ContextChunkInsertInput,
) (*ContextChunkRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if input.AccountID == uuid.Nil || input.ThreadID == uuid.Nil || input.AtomID == uuid.Nil {
		return nil, fmt.Errorf("account_id, thread_id and atom_id must not be empty")
	}
	if err := ensureChunkAtomOwnership(ctx, tx, input.AccountID, input.ThreadID, input.AtomID); err != nil {
		return nil, err
	}
	chunkSeq := input.ChunkSeq
	if chunkSeq <= 0 {
		chunkSeq = int64(input.ChunkIndex + 1)
	}
	if chunkSeq <= 0 || input.ContextSeq <= 0 {
		return nil, fmt.Errorf("chunk_seq and context_seq must be positive")
	}
	chunkKind := strings.TrimSpace(input.ChunkKind)
	if chunkKind == "" {
		chunkKind = "payload"
	}
	payloadJSON := input.PayloadJSON
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}
	metadataJSON := input.MetadataJSON
	if len(metadataJSON) == 0 {
		metadataJSON = json.RawMessage(`{}`)
	}

	payloadText := input.PayloadText
	if strings.TrimSpace(payloadText) == "" {
		payloadText = input.Text
	}

	var item ContextChunkRecord
	err := tx.QueryRow(
		ctx,
		`INSERT INTO thread_context_chunks (
			account_id, thread_id, atom_id, chunk_seq, context_seq, chunk_kind,
			payload_text, payload_json, metadata_json
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb
		)
		ON CONFLICT (thread_id, context_seq) DO UPDATE SET
			account_id = EXCLUDED.account_id,
			atom_id = EXCLUDED.atom_id,
			chunk_seq = EXCLUDED.chunk_seq,
			chunk_kind = EXCLUDED.chunk_kind,
			payload_text = EXCLUDED.payload_text,
			payload_json = EXCLUDED.payload_json,
			metadata_json = EXCLUDED.metadata_json
		RETURNING id, account_id, thread_id, atom_id, chunk_seq, context_seq, chunk_kind,
		          payload_text, payload_json, metadata_json, created_at`,
		input.AccountID,
		input.ThreadID,
		input.AtomID,
		chunkSeq,
		input.ContextSeq,
		chunkKind,
		payloadText,
		string(payloadJSON),
		string(metadataJSON),
	).Scan(
		&item.ID,
		&item.AccountID,
		&item.ThreadID,
		&item.AtomID,
		&item.ChunkSeq,
		&item.ContextSeq,
		&item.ChunkKind,
		&item.PayloadText,
		&item.PayloadJSON,
		&item.MetadataJSON,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.ChunkKind = strings.TrimSpace(item.ChunkKind)
	item.ChunkIndex = int(item.ChunkSeq - 1)
	item.Text = item.PayloadText
	item.PayloadText = strings.TrimSpace(item.PayloadText)
	return &item, nil
}

func (ThreadContextChunksRepository) ListByThreadUpToContextSeq(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	upperBoundContextSeq *int64,
) ([]ContextChunkRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return nil, fmt.Errorf("account_id and thread_id must not be empty")
	}

	args := []any{accountID, threadID}
	query := `SELECT id, account_id, thread_id, atom_id, chunk_seq, context_seq, chunk_kind,
	                 payload_text, payload_json, metadata_json, created_at
	            FROM thread_context_chunks
	           WHERE account_id = $1
	             AND thread_id = $2`
	if upperBoundContextSeq != nil {
		query += ` AND context_seq <= $3`
		args = append(args, *upperBoundContextSeq)
	}
	query += ` ORDER BY context_seq ASC`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ContextChunkRecord, 0)
	for rows.Next() {
		var item ContextChunkRecord
		if err := rows.Scan(
			&item.ID,
			&item.AccountID,
			&item.ThreadID,
			&item.AtomID,
			&item.ChunkSeq,
			&item.ContextSeq,
			&item.ChunkKind,
			&item.PayloadText,
			&item.PayloadJSON,
			&item.MetadataJSON,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.ChunkKind = strings.TrimSpace(item.ChunkKind)
		item.ChunkIndex = int(item.ChunkSeq - 1)
		item.Text = item.PayloadText
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (ThreadContextChunksRepository) GetContextSeqRangeForChunkIDs(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	chunkIDs []uuid.UUID,
) (int64, int64, error) {
	if tx == nil {
		return 0, 0, fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return 0, 0, fmt.Errorf("account_id and thread_id must not be empty")
	}
	if len(chunkIDs) == 0 {
		return 0, 0, fmt.Errorf("chunk_ids must not be empty")
	}
	var startContextSeq int64
	var endContextSeq int64
	err := tx.QueryRow(
		ctx,
		`SELECT MIN(context_seq), MAX(context_seq)
		   FROM thread_context_chunks
		  WHERE account_id = $1
		    AND thread_id = $2
		    AND id = ANY($3::uuid[])`,
		accountID,
		threadID,
		chunkIDs,
	).Scan(&startContextSeq, &endContextSeq)
	if err != nil {
		return 0, 0, err
	}
	if startContextSeq <= 0 || endContextSeq <= 0 {
		return 0, 0, fmt.Errorf("context seq range not found")
	}
	return startContextSeq, endContextSeq, nil
}

func (ThreadContextChunksRepository) DeleteByThreadContextSeq(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	contextSeq int64,
) error {
	if tx == nil {
		return fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return fmt.Errorf("account_id and thread_id must not be empty")
	}
	if contextSeq <= 0 {
		return fmt.Errorf("context_seq must be positive")
	}
	if _, err := tx.Exec(
		ctx,
		`DELETE FROM thread_context_chunks
		  WHERE account_id = $1
		    AND thread_id = $2
		    AND context_seq = $3`,
		accountID,
		threadID,
		contextSeq,
	); err != nil {
		return err
	}
	return nil
}

func ensureChunkAtomOwnership(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	atomID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			  FROM thread_context_atoms
			 WHERE id = $1
			   AND account_id = $2
			   AND thread_id = $3
		)`,
		atomID,
		accountID,
		threadID,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("atom_id does not belong to account/thread")
	}
	return nil
}

func validateChunkAtomOwnership(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	atomID uuid.UUID,
) error {
	var atomAccountID uuid.UUID
	var atomThreadID uuid.UUID
	err := tx.QueryRow(
		ctx,
		`SELECT account_id, thread_id
		   FROM thread_context_atoms
		  WHERE id = $1`,
		atomID,
	).Scan(&atomAccountID, &atomThreadID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("atom_id not found")
		}
		return err
	}
	if atomAccountID != accountID || atomThreadID != threadID {
		return fmt.Errorf("atom_id does not belong to the provided account/thread")
	}
	return nil
}
