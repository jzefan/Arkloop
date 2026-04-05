package pipeline

import (
	"context"
	"fmt"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/routing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type impressionTestGateway struct{}

func (impressionTestGateway) Stream(_ context.Context, _ llm.Request, _ func(llm.StreamEvent) error) error {
	return nil
}

type impressionTestRow struct {
	values []any
	err    error
}

func (r impressionTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch ptr := dest[i].(type) {
		case *string:
			value, ok := r.values[i].(string)
			if !ok {
				return fmt.Errorf("unexpected value type %T", r.values[i])
			}
			*ptr = value
		default:
			return fmt.Errorf("unexpected scan target %T", dest[i])
		}
	}
	return nil
}

type impressionTestDB struct {
	selector string
}

func (db impressionTestDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db impressionTestDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if db.selector == "" {
		return impressionTestRow{err: pgx.ErrNoRows}
	}
	return impressionTestRow{values: []any{db.selector}}
}

func (db impressionTestDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (db impressionTestDB) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return nil, fmt.Errorf("BeginTx should not be called in this test")
}

func TestImpressionPrepareMiddlewareUsesAccountToolRoute(t *testing.T) {
	routeCfg := routing.ProviderRoutingConfig{
		Credentials: []routing.ProviderCredential{
			{
				ID:           "cred-tool",
				Name:         "tool-cred",
				OwnerKind:    routing.CredentialScopePlatform,
				ProviderKind: routing.ProviderKindStub,
			},
		},
		Routes: []routing.ProviderRouteRule{
			{
				ID:           "route-chat",
				Model:        "chat-model",
				CredentialID: "cred-tool",
			},
			{
				ID:           "route-tool",
				Model:        "tool-model",
				CredentialID: "cred-tool",
				Priority:     10,
			},
		},
	}
	loader := routing.NewDesktopSQLiteRoutingLoader(func(context.Context) (routing.ProviderRoutingConfig, error) {
		return routeCfg, nil
	}, routing.ProviderRoutingConfig{})
	auxGateway := impressionTestGateway{}

	mw := NewImpressionPrepareMiddleware(nil, impressionTestDB{selector: "tool-cred^tool-model"}, auxGateway, false, loader)

	uid := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
		},
		InputJSON: map[string]any{
			"run_kind": "impression",
		},
		UserID:  &uid,
		Gateway: auxGateway,
		SelectedRoute: &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    "route-chat",
				Model: "chat-model",
			},
			Credential: routeCfg.Credentials[0],
		},
		RoutingByokEnabled: true,
	}

	err := mw(context.Background(), rc, func(_ context.Context, inner *RunContext) error {
		if inner.Gateway == nil {
			t.Fatal("expected gateway override")
		}
		if inner.SelectedRoute == nil {
			t.Fatal("expected selected route override")
		}
		if inner.SelectedRoute.Route.ID != "route-tool" {
			t.Fatalf("got route id %q, want %q", inner.SelectedRoute.Route.ID, "route-tool")
		}
		if inner.SelectedRoute.Route.Model != "tool-model" {
			t.Fatalf("got model %q, want %q", inner.SelectedRoute.Route.Model, "tool-model")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
