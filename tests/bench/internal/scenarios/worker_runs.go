package scenarios

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"arkloop/tests/bench/internal/httpx"
	"arkloop/tests/bench/internal/report"
	"arkloop/tests/bench/internal/stats"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkerRunsConfig struct {
	APIBaseURL string
	Token      string
	DBDSN      string

	RunCount  int
	Timeout   time.Duration
	Threshold WorkerRunsThresholds
}

type WorkerRunsThresholds struct {
	MaxFailedRate   float64
	TimeoutRateZero bool
}

func DefaultWorkerRunsConfig(apiBaseURL, token string) WorkerRunsConfig {
	return WorkerRunsConfig{
		APIBaseURL: apiBaseURL,
		Token:      token,
		RunCount:   50,
		Timeout:    60 * time.Second,
		Threshold: WorkerRunsThresholds{
			MaxFailedRate:   0.01,
			TimeoutRateZero: true,
		},
	}
}

func RunWorkerRuns(ctx context.Context, cfg WorkerRunsConfig) report.ScenarioResult {
	result := report.ScenarioResult{
		Name:       "worker_runs",
		Config:     map[string]any{},
		Stats:      map[string]any{},
		Thresholds: map[string]any{},
		Pass:       false,
	}

	result.Config["run_count"] = cfg.RunCount
	result.Config["timeout_s"] = cfg.Timeout.Seconds()
	result.Thresholds["max_failed_rate"] = cfg.Threshold.MaxFailedRate
	result.Thresholds["timeout_rate_zero"] = cfg.Threshold.TimeoutRateZero

	if cfg.Token == "" {
		result.Errors = append(result.Errors, "auth.missing_token")
		return result
	}
	if cfg.RunCount <= 0 {
		result.Errors = append(result.Errors, "config.invalid")
		return result
	}

	client := httpx.NewClient(2 * time.Second)
	sseClient := httpx.NewNoTimeoutClient()
	headers := map[string]string{
		"Authorization": "Bearer " + cfg.Token,
	}

	threads := make([]string, 0, cfg.RunCount)
	for i := 0; i < cfg.RunCount; i++ {
		threadID, errCode := createThread(ctx, client, cfg.APIBaseURL, headers)
		if errCode != "" {
			result.Errors = append(result.Errors, errCode)
			return result
		}
		threads = append(threads, threadID)
	}

	type createdRun struct {
		RunID     string
		StartedAt time.Time
	}

	runCh := make(chan createdRun, cfg.RunCount)
	var created int64
	var createFail int64
	errSet := sync.Map{}

	var createWG sync.WaitGroup
	for _, tid := range threads {
		createWG.Add(1)
		go func(threadID string) {
			defer createWG.Done()
			runID, errCode := createRun(ctx, client, cfg.APIBaseURL, threadID, headers, "lite")
			if errCode != "" {
				atomic.AddInt64(&createFail, 1)
				_, _ = errSet.LoadOrStore("worker_runs."+errCode, struct{}{})
				return
			}
			start := time.Now()
			atomic.AddInt64(&created, 1)
			runCh <- createdRun{RunID: runID, StartedAt: start}
		}(tid)
	}
	createWG.Wait()
	close(runCh)

	createdRuns := make([]createdRun, 0, cfg.RunCount)
	for item := range runCh {
		createdRuns = append(createdRuns, item)
	}

	var completed int64
	var failed int64
	var cancelled int64
	var timedOut int64

	completionMs := make([]float64, 0, len(createdRuns))
	var completionMu sync.Mutex

	var maxTotalConns int64
	var maxActiveConns int64
	monitorStop := make(chan struct{})
	cancelMonitor := func() {}
	if strings.TrimSpace(cfg.DBDSN) != "" {
		monitorCtx, cancel := context.WithCancel(ctx)
		cancelMonitor = cancel
		go func() {
			defer close(monitorStop)
			pool, err := pgxpool.New(monitorCtx, cfg.DBDSN)
			if err != nil {
				_, _ = errSet.LoadOrStore("worker_runs.db.connect_error", struct{}{})
				return
			}
			defer pool.Close()

			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-monitorCtx.Done():
					return
				case <-ticker.C:
					var total int64
					var active int64
					q := `
						SELECT
						  COUNT(*)::bigint AS total,
						  COUNT(*) FILTER (WHERE state = 'active')::bigint AS active
						FROM pg_stat_activity
						WHERE datname = current_database()
					`
					qctx, qcancel := context.WithTimeout(monitorCtx, 500*time.Millisecond)
					err := pool.QueryRow(qctx, q).Scan(&total, &active)
					qcancel()
					if err != nil {
						continue
					}
					for {
						prev := atomic.LoadInt64(&maxTotalConns)
						if total <= prev || atomic.CompareAndSwapInt64(&maxTotalConns, prev, total) {
							break
						}
					}
					for {
						prev := atomic.LoadInt64(&maxActiveConns)
						if active <= prev || atomic.CompareAndSwapInt64(&maxActiveConns, prev, active) {
							break
						}
					}
				}
			}
		}()
	} else {
		close(monitorStop)
	}

	var wg sync.WaitGroup
	for _, run := range createdRuns {
		wg.Add(1)
		go func(item createdRun) {
			defer wg.Done()

			ctxRun, cancel := context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()

			eventsURL, err := httpx.JoinURL(cfg.APIBaseURL, "/v1/runs/"+item.RunID+"/events?follow=true&after_seq=0")
			if err != nil {
				atomic.AddInt64(&timedOut, 1)
				_, _ = errSet.LoadOrStore("worker_runs.config.invalid_base_url", struct{}{})
				return
			}

			req, _ := http.NewRequestWithContext(ctxRun, http.MethodGet, eventsURL, nil)
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
			req.Header.Set("Accept", "text/event-stream")

			resp, err := sseClient.Do(req)
			if err != nil {
				atomic.AddInt64(&timedOut, 1)
				_, _ = errSet.LoadOrStore("worker_runs.sse.net.error", struct{}{})
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				atomic.AddInt64(&timedOut, 1)
				_, _ = errSet.LoadOrStore("worker_runs.sse.http."+itoa(resp.StatusCode), struct{}{})
				return
			}

			eventType, err := waitTerminalEvent(ctxRun, resp.Body)
			if err != nil {
				atomic.AddInt64(&timedOut, 1)
				_, _ = errSet.LoadOrStore("worker_runs.sse.timeout", struct{}{})
				return
			}

			switch eventType {
			case "run.completed":
				atomic.AddInt64(&completed, 1)
			case "run.failed":
				atomic.AddInt64(&failed, 1)
			case "run.cancelled":
				atomic.AddInt64(&cancelled, 1)
			default:
				atomic.AddInt64(&failed, 1)
				_, _ = errSet.LoadOrStore("worker_runs.terminal.unknown", struct{}{})
			}

			ms := float64(time.Since(item.StartedAt).Nanoseconds()) / 1e6
			completionMu.Lock()
			completionMs = append(completionMs, ms)
			completionMu.Unlock()
		}(run)
	}

	wg.Wait()
	cancelMonitor()
	<-monitorStop

	createdN := atomic.LoadInt64(&created)
	createFailN := atomic.LoadInt64(&createFail)
	completedN := atomic.LoadInt64(&completed)
	failedN := atomic.LoadInt64(&failed)
	cancelledN := atomic.LoadInt64(&cancelled)
	timedOutN := atomic.LoadInt64(&timedOut)

	termTotal := completedN + failedN + cancelledN
	failedTotal := failedN + cancelledN

	failedRate := 0.0
	if createdN > 0 {
		failedRate = float64(failedTotal) / float64(createdN)
	}
	timeoutRate := 0.0
	if createdN > 0 {
		timeoutRate = float64(timedOutN) / float64(createdN)
	}

	result.Stats["runs_requested"] = cfg.RunCount
	result.Stats["runs_created"] = createdN
	result.Stats["runs_create_failed"] = createFailN
	result.Stats["runs_completed"] = completedN
	result.Stats["runs_failed"] = failedN
	result.Stats["runs_cancelled"] = cancelledN
	result.Stats["runs_timeout"] = timedOutN
	result.Stats["runs_terminal_total"] = termTotal
	result.Stats["failed_rate"] = failedRate
	result.Stats["timeout_rate"] = timeoutRate
	completionSummary, sumErr := stats.SummarizeMs(completionMs)
	if sumErr != nil {
		_, _ = errSet.LoadOrStore("worker_runs.completion.empty", struct{}{})
	}
	result.Stats["run_completion_ms"] = completionSummary
	result.Stats["pg_stat_activity_max_total"] = atomic.LoadInt64(&maxTotalConns)
	result.Stats["pg_stat_activity_max_active"] = atomic.LoadInt64(&maxActiveConns)

	errors := make([]string, 0, 16)
	errSet.Range(func(key, value any) bool {
		k, _ := key.(string)
		errors = append(errors, k)
		return true
	})
	result.Errors = append(result.Errors, errors...)

	pass := true
	if createdN != int64(cfg.RunCount) {
		pass = false
	}
	if cfg.Threshold.TimeoutRateZero && timedOutN > 0 {
		pass = false
	}
	if failedRate > cfg.Threshold.MaxFailedRate {
		pass = false
	}

	result.Pass = pass
	return result
}

func waitTerminalEvent(ctx context.Context, body io.Reader) (string, error) {
	reader := bufio.NewReader(body)
	var eventType string
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if line == "" {
			switch eventType {
			case "run.completed", "run.failed", "run.cancelled":
				return eventType, nil
			}
			eventType = ""
		}
	}
}
