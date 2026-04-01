package agent

import (
	"context"
	"time"

	"arkloop/services/worker/internal/events"
)

const (
	ErrorClassRunDeadlineExceeded  = "run.deadline_exceeded"
	ErrorClassRunPausedWaitingUser = "run.paused_waiting_user"
	EventTypeRunPaused             = "run.paused"
	EventTypeRunResumed            = "run.resumed"
)

type LoopGovernor struct {
	runCtx                RunContext
	startedAt             time.Time
	lastActivityAt        time.Time
	idleHeartbeatInterval time.Duration
}

func NewLoopGovernor(runCtx RunContext) *LoopGovernor {
	now := time.Now()
	return &LoopGovernor{
		runCtx:                runCtx,
		startedAt:             now,
		lastActivityAt:        now,
		idleHeartbeatInterval: runCtx.IdleHeartbeatInterval,
	}
}

func (g *LoopGovernor) Touch() {
	if g == nil {
		return
	}
	g.lastActivityAt = time.Now()
}

func (g *LoopGovernor) Check(ctx context.Context, emitter events.Emitter, yield func(events.RunEvent) error) (bool, error) {
	if g == nil {
		return false, nil
	}
	if runDeadlineExceeded(ctx) {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, nil
	}
	if g.runCtx.RunDeadline > 0 && time.Since(g.startedAt) >= g.runCtx.RunDeadline {
		return true, nil
	}
	if g.idleHeartbeatInterval > 0 && time.Since(g.lastActivityAt) >= g.idleHeartbeatInterval {
		g.lastActivityAt = time.Now()
		if err := yield(emitter.Emit("run.idle_heartbeat", map[string]any{
			"idle_ms": time.Since(g.startedAt).Milliseconds(),
		}, nil, nil)); err != nil {
			return false, err
		}
	}
	return false, nil
}

func (g *LoopGovernor) WaitForUserInput(
	ctx context.Context,
	emitter events.Emitter,
	yield func(events.RunEvent) error,
	requestID string,
	wait func(context.Context) (string, bool),
) (string, bool, bool, error) {
	if err := yield(emitter.Emit(EventTypeRunPaused, map[string]any{
		"reason":     "waiting_user_input",
		"request_id": requestID,
	}, nil, nil)); err != nil {
		return "", false, false, err
	}

	waitCtx := ctx
	cancel := func() {}
	if g != nil && g.runCtx.PausedInputTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, g.runCtx.PausedInputTimeout)
	}
	defer cancel()

	text, ok := wait(waitCtx)
	if !ok {
		return "", false, waitCtx.Err() == context.DeadlineExceeded, nil
	}
	g.Touch()
	if err := yield(emitter.Emit(EventTypeRunResumed, map[string]any{
		"reason":     "user_input_received",
		"request_id": requestID,
	}, nil, nil)); err != nil {
		return "", false, false, err
	}
	return text, true, false, nil
}
