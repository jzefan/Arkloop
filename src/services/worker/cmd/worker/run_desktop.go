//go:build desktop

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"arkloop/services/shared/eventbus"
	"arkloop/services/worker/internal/app"
	"arkloop/services/worker/internal/consumer"
	"arkloop/services/worker/internal/queue"
)

// run is the desktop entry point. It uses in-process adapters (LocalEventBus,
// ChannelJobQueue) and does not depend on PostgreSQL, Redis, or S3.
func run() error {
	if _, err := app.LoadDotenvIfEnabled(false); err != nil {
		return err
	}

	cfg, err := app.LoadConfigFromEnv()
	if err != nil {
		return err
	}

	logger := app.NewJSONLogger("worker_go", os.Stdout)

	bus := eventbus.NewLocalEventBus()
	defer bus.Close()

	localNotifier := consumer.NewLocalNotifier()
	cq, err := queue.NewChannelJobQueue(25, localNotifier.Notify)
	if err != nil {
		return err
	}

	logger.Info("desktop mode: using LocalEventBus and ChannelJobQueue", app.LogFields{}, nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := &desktopHandler{
		bus:    bus,
		queue:  cq,
		logger: logger,
	}

	loop, err := consumer.NewLoop(
		cq,
		handler,
		nil, // no advisory locker for desktop (single user, concurrency=1)
		consumer.Config{
			Concurrency:      1,
			PollSeconds:      cfg.PollSeconds,
			LeaseSeconds:     cfg.LeaseSeconds,
			HeartbeatSeconds: cfg.HeartbeatSeconds,
			QueueJobTypes:    cfg.QueueJobTypes,
		},
		logger,
		localNotifier,
	)
	if err != nil {
		return err
	}

	logger.Info("desktop worker entering consume mode", app.LogFields{}, map[string]any{
		"concurrency": 1,
		"job_types":   cfg.QueueJobTypes,
	})
	return loop.Run(ctx)
}

// desktopHandler processes jobs in desktop mode. It receives jobs from the
// ChannelJobQueue and publishes lifecycle events to the LocalEventBus.
type desktopHandler struct {
	bus    eventbus.EventBus
	queue  queue.JobQueue
	logger *app.JSONLogger
}

func (h *desktopHandler) Handle(ctx context.Context, lease queue.JobLease) error {
	jobType, _ := lease.PayloadJSON["type"].(string)
	traceID, _ := lease.PayloadJSON["trace_id"].(string)
	runID, _ := lease.PayloadJSON["run_id"].(string)
	orgID, _ := lease.PayloadJSON["org_id"].(string)

	fields := app.LogFields{
		JobID:   strPtr(lease.JobID.String()),
		TraceID: strPtr(traceID),
		RunID:   strPtr(runID),
		OrgID:   strPtr(orgID),
	}

	h.logger.Info("desktop handler received job", fields, map[string]any{"job_type": jobType})

	h.publishEvent(ctx, "worker.job.received", map[string]any{
		"job_id":   lease.JobID.String(),
		"job_type": jobType,
		"trace_id": traceID,
		"run_id":   runID,
		"org_id":   orgID,
	})

	// TODO: integrate desktop engine composition for full LLM pipeline processing

	h.publishEvent(ctx, "worker.job.completed", map[string]any{
		"job_id":   lease.JobID.String(),
		"job_type": jobType,
		"run_id":   runID,
	})

	h.logger.Info("desktop handler completed job", fields, nil)
	return nil
}

func (h *desktopHandler) publishEvent(ctx context.Context, topic string, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = h.bus.Publish(ctx, topic, string(data))
}

func strPtr(v string) *string {
	s := v
	return &s
}
