package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/pipeline"
)

func TestEntitlementNilResolverPassThrough(t *testing.T) {
	mw := pipeline.NewEntitlementMiddleware(nil, data.RunsRepository{}, data.RunEventsRepository{}, nil)

	rc := &pipeline.RunContext{
		Emitter: events.NewEmitter("test"),
	}

	var reached bool
	h := pipeline.Build([]pipeline.RunMiddleware{mw}, func(_ context.Context, _ *pipeline.RunContext) error {
		reached = true
		return nil
	})
	if err := h(context.Background(), rc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reached {
		t.Fatal("terminal handler was not called")
	}
}

func TestEntitlementNilResolverInChain(t *testing.T) {
	sentinel := errors.New("sentinel")

	first := func(_ context.Context, rc *pipeline.RunContext, next pipeline.RunHandler) error {
		rc.TraceID = "first-done"
		return next(context.Background(), rc)
	}
	entitlement := pipeline.NewEntitlementMiddleware(nil, data.RunsRepository{}, data.RunEventsRepository{}, nil)
	last := func(_ context.Context, rc *pipeline.RunContext, next pipeline.RunHandler) error {
		if rc.TraceID != "first-done" {
			t.Fatal("chain order violated: expected first-done marker")
		}
		return sentinel
	}

	h := pipeline.Build([]pipeline.RunMiddleware{first, entitlement, last}, func(_ context.Context, _ *pipeline.RunContext) error {
		t.Fatal("terminal should not be reached")
		return nil
	})

	rc := &pipeline.RunContext{
		Emitter: events.NewEmitter("test"),
	}
	err := h(context.Background(), rc)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}
