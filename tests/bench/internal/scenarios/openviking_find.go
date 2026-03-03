package scenarios

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"arkloop/tests/bench/internal/httpx"
	"arkloop/tests/bench/internal/report"
	"arkloop/tests/bench/internal/stats"
)

type OpenVikingFindConfig struct {
	BaseURL     string
	RootAPIKey  string
	Warmup      time.Duration
	Duration    time.Duration
	Concurrency int
	Threshold   OpenVikingFindThresholds
}

type OpenVikingFindThresholds struct {
	P99Ms float64
}

func DefaultOpenVikingFindConfig(baseURL, rootKey string) OpenVikingFindConfig {
	return OpenVikingFindConfig{
		BaseURL:     baseURL,
		RootAPIKey:  rootKey,
		Warmup:      5 * time.Second,
		Duration:    30 * time.Second,
		Concurrency: 100,
		Threshold: OpenVikingFindThresholds{
			P99Ms: 500,
		},
	}
}

func RunOpenVikingFind(ctx context.Context, cfg OpenVikingFindConfig) report.ScenarioResult {
	result := report.ScenarioResult{
		Name:       "openviking_find",
		Config:     map[string]any{},
		Stats:      map[string]any{},
		Thresholds: map[string]any{},
		Pass:       false,
	}

	result.Config["warmup_s"] = cfg.Warmup.Seconds()
	result.Config["duration_s"] = cfg.Duration.Seconds()
	result.Config["concurrency"] = cfg.Concurrency
	result.Thresholds["p99_ms"] = cfg.Threshold.P99Ms

	if cfg.RootAPIKey == "" {
		result.Errors = append(result.Errors, "openviking.missing_root_key")
		return result
	}
	if cfg.Concurrency <= 0 {
		result.Errors = append(result.Errors, "config.invalid")
		return result
	}

	u, err := httpx.JoinURL(cfg.BaseURL, "/api/v1/search/find")
	if err != nil {
		result.Errors = append(result.Errors, "config.invalid_base_url")
		return result
	}

	client := httpx.NewClient(2 * time.Second)

	// warmup
	_, _ = runOpenVikingFindPhase(ctx, client, u, cfg.RootAPIKey, cfg.Concurrency, cfg.Warmup)

	phase, errs := runOpenVikingFindPhase(ctx, client, u, cfg.RootAPIKey, cfg.Concurrency, cfg.Duration)
	if len(errs) > 0 {
		result.Errors = append(result.Errors, errs...)
	}

	lat, sumErr := stats.SummarizeMs(phase.LatenciesMs)
	if sumErr != nil {
		result.Errors = append(result.Errors, "openviking.latency.empty")
	}
	var rate2xx float64
	if phase.TotalResponses > 0 {
		rate2xx = float64(phase.Status2xx) / float64(phase.TotalResponses)
	}

	result.Stats["latency_ms"] = lat
	result.Stats["responses_total"] = phase.TotalResponses
	result.Stats["responses_2xx"] = phase.Status2xx
	result.Stats["rate_2xx"] = rate2xx
	result.Stats["status_codes"] = phase.StatusCounts

	result.Pass = lat.Count > 0 && lat.P99Ms > 0 && lat.P99Ms < cfg.Threshold.P99Ms
	return result
}

type openvikingPhaseStats struct {
	LatenciesMs    []float64
	StatusCounts   map[string]int64
	TotalResponses int64
	Status2xx      int64
}

func runOpenVikingFindPhase(
	ctx context.Context,
	client *http.Client,
	url string,
	rootKey string,
	concurrency int,
	duration time.Duration,
) (openvikingPhaseStats, []string) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, duration+2*time.Second)
	defer cancel()

	var total int64
	var ok2xx int64
	statusCounts := map[string]int64{}
	var statusMu sync.Mutex
	errSet := sync.Map{}

	type workerStats struct {
		lat []float64
	}
	workers := make([]workerStats, concurrency)

	end := time.Now().Add(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for time.Now().Before(end) {
				headers := map[string]string{
					"X-API-Key":            rootKey,
					"X-OpenViking-Account": UUIDString(),
					"X-OpenViking-User":    UUIDString(),
					"X-OpenViking-Agent":   "bench",
				}
				body := map[string]any{
					"query":      "bench",
					"target_uri": "viking://user/" + UUIDString() + "/memories/",
					"limit":      3,
				}

				start := time.Now()
				err := httpx.DoJSON(ctx, client, http.MethodPost, url, headers, body, nil)
				latMs := float64(time.Since(start).Nanoseconds()) / 1e6
				atomic.AddInt64(&total, 1)

				statusKey := "http.200"
				if err == nil {
					atomic.AddInt64(&ok2xx, 1)
					workers[idx].lat = append(workers[idx].lat, latMs)
				} else if httpErr, ok := err.(*httpx.HTTPError); ok {
					statusKey = "http." + itoa(httpErr.Status)
					_, _ = errSet.LoadOrStore("openviking.http."+itoa(httpErr.Status), struct{}{})
				} else {
					statusKey = "net.error"
					_, _ = errSet.LoadOrStore("openviking.net.error", struct{}{})
				}

				statusMu.Lock()
				statusCounts[statusKey]++
				statusMu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	latencies := make([]float64, 0, total)
	for _, w := range workers {
		latencies = append(latencies, w.lat...)
	}

	errors := make([]string, 0, 8)
	errSet.Range(func(key, value any) bool {
		k, _ := key.(string)
		errors = append(errors, k)
		return true
	})

	return openvikingPhaseStats{
		LatenciesMs:    latencies,
		StatusCounts:   statusCounts,
		TotalResponses: atomic.LoadInt64(&total),
		Status2xx:      atomic.LoadInt64(&ok2xx),
	}, errors
}
