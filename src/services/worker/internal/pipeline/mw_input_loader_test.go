package pipeline_test

import (
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/pipeline"
)

func TestInputLoaderConstructorDoesNotPanic(t *testing.T) {
	mw := pipeline.NewInputLoaderMiddleware(data.RunEventsRepository{}, data.MessagesRepository{})
	if mw == nil {
		t.Fatal("expected non-nil middleware")
	}
}
