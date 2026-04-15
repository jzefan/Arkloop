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

type ProtocolAtomRecord struct {
	ID                    uuid.UUID
	AccountID             uuid.UUID
	ThreadID              uuid.UUID
	AtomSeq               int64
	AtomIndex             int64
	AtomKind              string
	Role                  string
	SourceMessageStartSeq int64
	SourceMessageEndSeq   int64
	StartThreadSeq        int64
	EndThreadSeq          int64
	PayloadText           string
	PayloadJSON           json.RawMessage
	MetadataJSON          json.RawMessage
	CreatedAt             time.Time
}

type ProtocolAtomInsertInput struct {
	AccountID             uuid.UUID
	ThreadID              uuid.UUID
	AtomSeq               int64
	AtomIndex             int64
	AtomKind              string
	Role                  string
	SourceMessageStartSeq int64
	SourceMessageEndSeq   int64
	StartThreadSeq        int64
	EndThreadSeq          int64
	PayloadText           string
	PayloadJSON           json.RawMessage
	MetadataJSON          json.RawMessage
}

type ThreadContextAtomsRepository struct{}

func (ThreadContextAtomsRepository) Insert(
	ctx context.Context,
	tx pgx.Tx,
	input ProtocolAtomInsertInput,
) (*ProtocolAtomRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if input.AccountID == uuid.Nil || input.ThreadID == uuid.Nil {
		return nil, fmt.Errorf("account_id and thread_id must not be empty")
	}
	atomSeq := input.AtomSeq
	if atomSeq <= 0 {
		atomSeq = input.AtomIndex
	}
	if atomSeq <= 0 {
		return nil, fmt.Errorf("atom_seq must be positive")
	}
	atomKind := strings.TrimSpace(input.AtomKind)
	if atomKind == "" {
		return nil, fmt.Errorf("atom_kind must not be empty")
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		return nil, fmt.Errorf("role must not be empty")
	}
	startThreadSeq := input.SourceMessageStartSeq
	endThreadSeq := input.SourceMessageEndSeq
	if startThreadSeq <= 0 {
		startThreadSeq = input.StartThreadSeq
	}
	if endThreadSeq <= 0 {
		endThreadSeq = input.EndThreadSeq
	}
	if startThreadSeq <= 0 || endThreadSeq <= 0 || startThreadSeq > endThreadSeq {
		return nil, fmt.Errorf("invalid source message seq range")
	}
	payloadJSON := input.PayloadJSON
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}
	metadataJSON := input.MetadataJSON
	if len(metadataJSON) == 0 {
		metadataJSON = json.RawMessage(`{}`)
	}

	var item ProtocolAtomRecord
	err := tx.QueryRow(
		ctx,
		`INSERT INTO thread_context_atoms (
			account_id, thread_id, atom_seq, atom_kind, role,
			source_message_start_seq, source_message_end_seq,
			payload_text, payload_json, metadata_json
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb
		)
		ON CONFLICT (thread_id, atom_seq) DO UPDATE SET
			atom_kind = EXCLUDED.atom_kind,
			role = EXCLUDED.role,
			source_message_start_seq = EXCLUDED.source_message_start_seq,
			source_message_end_seq = EXCLUDED.source_message_end_seq,
			payload_text = EXCLUDED.payload_text,
			payload_json = EXCLUDED.payload_json,
			metadata_json = EXCLUDED.metadata_json
		RETURNING id, account_id, thread_id, atom_seq, atom_kind, role,
		          source_message_start_seq, source_message_end_seq,
		          payload_text, payload_json, metadata_json, created_at`,
		input.AccountID,
		input.ThreadID,
		atomSeq,
		atomKind,
		role,
		startThreadSeq,
		endThreadSeq,
		input.PayloadText,
		string(payloadJSON),
		string(metadataJSON),
	).Scan(
		&item.ID,
		&item.AccountID,
		&item.ThreadID,
		&item.AtomSeq,
		&item.AtomKind,
		&item.Role,
		&item.SourceMessageStartSeq,
		&item.SourceMessageEndSeq,
		&item.PayloadText,
		&item.PayloadJSON,
		&item.MetadataJSON,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.AtomKind = strings.TrimSpace(item.AtomKind)
	item.Role = strings.TrimSpace(item.Role)
	item.AtomIndex = item.AtomSeq
	item.StartThreadSeq = item.SourceMessageStartSeq
	item.EndThreadSeq = item.SourceMessageEndSeq
	item.PayloadText = strings.TrimSpace(item.PayloadText)
	return &item, nil
}

func (ThreadContextAtomsRepository) Upsert(
	ctx context.Context,
	tx pgx.Tx,
	input ProtocolAtomInsertInput,
) (*ProtocolAtomRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if input.AccountID == uuid.Nil || input.ThreadID == uuid.Nil {
		return nil, fmt.Errorf("account_id and thread_id must not be empty")
	}
	atomSeq := input.AtomSeq
	if atomSeq <= 0 {
		atomSeq = input.AtomIndex
	}
	if atomSeq <= 0 {
		return nil, fmt.Errorf("atom_seq must be positive")
	}
	atomKind := strings.TrimSpace(input.AtomKind)
	if atomKind == "" {
		return nil, fmt.Errorf("atom_kind must not be empty")
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		return nil, fmt.Errorf("role must not be empty")
	}
	startThreadSeq := input.SourceMessageStartSeq
	endThreadSeq := input.SourceMessageEndSeq
	if startThreadSeq <= 0 {
		startThreadSeq = input.StartThreadSeq
	}
	if endThreadSeq <= 0 {
		endThreadSeq = input.EndThreadSeq
	}
	if startThreadSeq <= 0 || endThreadSeq <= 0 || startThreadSeq > endThreadSeq {
		return nil, fmt.Errorf("invalid source message seq range")
	}
	payloadJSON := input.PayloadJSON
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}
	metadataJSON := input.MetadataJSON
	if len(metadataJSON) == 0 {
		metadataJSON = json.RawMessage(`{}`)
	}

	var item ProtocolAtomRecord
	err := tx.QueryRow(
		ctx,
		`INSERT INTO thread_context_atoms (
			account_id, thread_id, atom_seq, atom_kind, role,
			source_message_start_seq, source_message_end_seq,
			payload_text, payload_json, metadata_json
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10::jsonb
		)
		ON CONFLICT (thread_id, atom_seq) DO UPDATE SET
			account_id = EXCLUDED.account_id,
			atom_kind = EXCLUDED.atom_kind,
			role = EXCLUDED.role,
			source_message_start_seq = EXCLUDED.source_message_start_seq,
			source_message_end_seq = EXCLUDED.source_message_end_seq,
			payload_text = EXCLUDED.payload_text,
			payload_json = EXCLUDED.payload_json,
			metadata_json = EXCLUDED.metadata_json
		RETURNING id, account_id, thread_id, atom_seq, atom_kind, role,
		          source_message_start_seq, source_message_end_seq,
		          payload_text, payload_json, metadata_json, created_at`,
		input.AccountID,
		input.ThreadID,
		atomSeq,
		atomKind,
		role,
		startThreadSeq,
		endThreadSeq,
		input.PayloadText,
		string(payloadJSON),
		string(metadataJSON),
	).Scan(
		&item.ID,
		&item.AccountID,
		&item.ThreadID,
		&item.AtomSeq,
		&item.AtomKind,
		&item.Role,
		&item.SourceMessageStartSeq,
		&item.SourceMessageEndSeq,
		&item.PayloadText,
		&item.PayloadJSON,
		&item.MetadataJSON,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	item.AtomKind = strings.TrimSpace(item.AtomKind)
	item.Role = strings.TrimSpace(item.Role)
	item.AtomIndex = item.AtomSeq
	item.StartThreadSeq = item.SourceMessageStartSeq
	item.EndThreadSeq = item.SourceMessageEndSeq
	item.PayloadText = strings.TrimSpace(item.PayloadText)
	return &item, nil
}

func (ThreadContextAtomsRepository) ListByThreadUpToAtomSeq(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	upperBoundAtomSeq *int64,
) ([]ProtocolAtomRecord, error) {
	if tx == nil {
		return nil, fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return nil, fmt.Errorf("account_id and thread_id must not be empty")
	}

	args := []any{accountID, threadID}
	query := `SELECT id, account_id, thread_id, atom_seq, atom_kind, role,
	                 source_message_start_seq, source_message_end_seq,
	                 payload_text, payload_json, metadata_json, created_at
	            FROM thread_context_atoms
	           WHERE account_id = $1
	             AND thread_id = $2`
	if upperBoundAtomSeq != nil {
		query += ` AND atom_seq <= $3`
		args = append(args, *upperBoundAtomSeq)
	}
	query += ` ORDER BY atom_seq ASC`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ProtocolAtomRecord, 0)
	for rows.Next() {
		var item ProtocolAtomRecord
		if err := rows.Scan(
			&item.ID,
			&item.AccountID,
			&item.ThreadID,
			&item.AtomSeq,
			&item.AtomKind,
			&item.Role,
			&item.SourceMessageStartSeq,
			&item.SourceMessageEndSeq,
			&item.PayloadText,
			&item.PayloadJSON,
			&item.MetadataJSON,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.AtomKind = strings.TrimSpace(item.AtomKind)
		item.Role = strings.TrimSpace(item.Role)
		item.AtomIndex = item.AtomSeq
		item.StartThreadSeq = item.SourceMessageStartSeq
		item.EndThreadSeq = item.SourceMessageEndSeq
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (ThreadContextAtomsRepository) DeleteByThreadAtomSeq(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	atomSeq int64,
) error {
	if tx == nil {
		return fmt.Errorf("tx must not be nil")
	}
	if accountID == uuid.Nil || threadID == uuid.Nil {
		return fmt.Errorf("account_id and thread_id must not be empty")
	}
	if atomSeq <= 0 {
		return fmt.Errorf("atom_seq must be positive")
	}
	if _, err := tx.Exec(
		ctx,
		`DELETE FROM thread_context_atoms
		  WHERE account_id = $1
		    AND thread_id = $2
		    AND atom_seq = $3`,
		accountID,
		threadID,
		atomSeq,
	); err != nil {
		return err
	}
	return nil
}
