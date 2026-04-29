package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCurrentChannelDefaultModel(t *testing.T) {
	channelID := uuid.New()
	db := heartbeatChannelConfigDB{raw: json.RawMessage(`{"default_model":"opencode go^deepseek-v4-pro"}`)}

	got := currentChannelDefaultModel(context.Background(), db, channelID)
	if got != "opencode go^deepseek-v4-pro" {
		t.Fatalf("default model = %q", got)
	}
}

func TestCurrentChannelDefaultModelReturnsEmptyOnInvalidConfig(t *testing.T) {
	channelID := uuid.New()
	db := heartbeatChannelConfigDB{raw: json.RawMessage(`{`)}

	if got := currentChannelDefaultModel(context.Background(), db, channelID); got != "" {
		t.Fatalf("default model = %q", got)
	}
}

type heartbeatChannelConfigDB struct {
	raw json.RawMessage
	err error
}

func (db heartbeatChannelConfigDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}

func (db heartbeatChannelConfigDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (db heartbeatChannelConfigDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return heartbeatChannelConfigRow(db)
}

func (db heartbeatChannelConfigDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.New("not implemented")
}

type heartbeatChannelConfigRow struct {
	raw json.RawMessage
	err error
}

func (r heartbeatChannelConfigRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("unexpected scan destination count")
	}
	raw, ok := dest[0].(*json.RawMessage)
	if !ok {
		return errors.New("unexpected scan destination")
	}
	*raw = append((*raw)[:0], r.raw...)
	return nil
}
