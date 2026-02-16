package runengine

import (
	"context"
	"strings"

	"arkloop/services/worker_go/internal/data"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	cancelEventTypes   = []string{"run.cancel_requested", "run.cancelled"}
	terminalEventTypes = []string{"run.completed", "run.failed", "run.cancelled"}
)

type EngineV1 struct {
	events   data.RunEventsRepository
	messages data.MessagesRepository
}

func NewEngineV1() *EngineV1 {
	return &EngineV1{}
}

type ExecuteInput struct {
	TraceID string
}

func (e *EngineV1) Execute(ctx context.Context, tx pgx.Tx, run data.Run, input ExecuteInput) error {
	cancelType, err := e.events.GetLatestEventType(ctx, tx, run.ID, cancelEventTypes)
	if err != nil {
		return err
	}
	if cancelType == "run.cancelled" {
		return nil
	}
	if cancelType == "run.cancel_requested" {
		_, err := e.events.AppendEvent(
			ctx,
			tx,
			run.ID,
			"run.cancelled",
			map[string]any{"trace_id": strings.TrimSpace(input.TraceID)},
			nil,
			nil,
		)
		return err
	}

	terminalType, err := e.events.GetLatestEventType(ctx, tx, run.ID, terminalEventTypes)
	if err != nil {
		return err
	}
	if terminalType != "" {
		return nil
	}

	routeID, err := e.resolveRouteID(ctx, tx, run.ID)
	if err != nil {
		return err
	}

	_, err = e.events.AppendEvent(
		ctx,
		tx,
		run.ID,
		"run.route.selected",
		map[string]any{"route_id": routeID},
		nil,
		nil,
	)
	if err != nil {
		return err
	}

	deltas := []string{
		"go native delta 1",
		"go native delta 2",
	}
	for _, delta := range deltas {
		_, err := e.events.AppendEvent(
			ctx,
			tx,
			run.ID,
			"message.delta",
			map[string]any{"content_delta": delta, "role": "assistant"},
			nil,
			nil,
		)
		if err != nil {
			return err
		}
	}

	content := strings.Join(deltas, "")
	if err := e.messages.InsertAssistantMessage(ctx, tx, run.OrgID, run.ThreadID, content); err != nil {
		return err
	}

	_, err = e.events.AppendEvent(ctx, tx, run.ID, "run.completed", map[string]any{}, nil, nil)
	return err
}

func (e *EngineV1) resolveRouteID(ctx context.Context, tx pgx.Tx, runID uuid.UUID) (string, error) {
	eventType, dataJSON, err := e.events.FirstEventData(ctx, tx, runID)
	if err != nil {
		return "", err
	}
	if eventType != "run.started" || dataJSON == nil {
		return "default", nil
	}
	rawRouteID, ok := dataJSON["route_id"]
	if !ok {
		return "default", nil
	}
	routeID, ok := rawRouteID.(string)
	if !ok {
		return "default", nil
	}
	cleaned := strings.TrimSpace(routeID)
	if cleaned == "" {
		return "default", nil
	}
	return cleaned, nil
}
