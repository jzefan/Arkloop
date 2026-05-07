package scenarios

import (
	"context"
	"time"

	"arkloop/tests/bench/internal/httpx"
	"arkloop/tests/bench/internal/report"
	"arkloop/tests/bench/internal/stats"
)

// GatewayAuthConfig 认证网关场景配置。
type GatewayAuthConfig struct {
	BaseURL      string
	AuthHeader   string // "Bearer <token>" 或 "Bearer ak-..."
	ScenarioName string
	Warmup       time.Duration
	Duration     time.Duration
	QPS          int
	Workers      int
	Timeout      time.Duration
	Threshold    GatewayThresholds
}

func DefaultGatewayJWTConfig(baseURL string, jwtToken string) GatewayAuthConfig {
	return GatewayAuthConfig{
		BaseURL:      baseURL,
		AuthHeader:   "Bearer " + jwtToken,
		ScenarioName: "gateway_jwt",
		Warmup:       5 * time.Second,
		Duration:     30 * time.Second,
		QPS:          1000,
		Workers:      200,
		Timeout:      2 * time.Second,
		Threshold: GatewayThresholds{
			P99Ms:      10,
			Min2xxRate: 0.999,
		},
	}
}

func DefaultGatewayAPIKeyConfig(baseURL string, apiKey string) GatewayAuthConfig {
	return GatewayAuthConfig{
		BaseURL:      baseURL,
		AuthHeader:   "Bearer " + apiKey,
		ScenarioName: "gateway_apikey",
		Warmup:       5 * time.Second,
		Duration:     30 * time.Second,
		QPS:          1000,
		Workers:      200,
		Timeout:      2 * time.Second,
		Threshold: GatewayThresholds{
			P99Ms:      15,
			Min2xxRate: 0.999,
		},
	}
}

// RunGatewayAuth 运行带认证的 gateway 压测场景。
func RunGatewayAuth(ctx context.Context, cfg GatewayAuthConfig) report.ScenarioResult {
	result := report.ScenarioResult{
		Name:       cfg.ScenarioName,
		Config:     map[string]any{},
		Stats:      map[string]any{},
		Thresholds: map[string]any{},
		Pass:       false,
	}

	result.Config["warmup_s"] = cfg.Warmup.Seconds()
	result.Config["duration_s"] = cfg.Duration.Seconds()
	result.Config["qps"] = cfg.QPS
	result.Config["workers"] = cfg.Workers
	result.Config["timeout_s"] = cfg.Timeout.Seconds()

	result.Thresholds["p99_ms"] = cfg.Threshold.P99Ms
	result.Thresholds["min_2xx_rate"] = cfg.Threshold.Min2xxRate

	if cfg.QPS <= 0 || cfg.Workers <= 0 {
		result.Errors = append(result.Errors, "config.invalid")
		return result
	}

	u, err := httpx.JoinURL(cfg.BaseURL, "/benchz")
	if err != nil {
		result.Errors = append(result.Errors, "config.invalid_base_url")
		return result
	}

	client := httpx.NewClient(cfg.Timeout)

	headers := map[string]string{
		"Authorization": cfg.AuthHeader,
	}

	// warmup
	_, _ = runGatewayPhase(ctx, client, u, cfg.QPS, cfg.Workers, cfg.Warmup, headers)

	// measure
	phase, errs := runGatewayPhase(ctx, client, u, cfg.QPS, cfg.Workers, cfg.Duration, headers)
	if len(errs) > 0 {
		result.Errors = append(result.Errors, errs...)
	}

	latSummary, sumErr := stats.SummarizeMs(phase.LatenciesMs)
	if sumErr != nil {
		result.Errors = append(result.Errors, cfg.ScenarioName+".latency.empty")
	}
	errLatSummary, _ := stats.SummarizeMs(phase.ErrorLatenciesMs)

	var rate2xx float64
	if phase.TotalResponses > 0 {
		rate2xx = float64(phase.Status2xx) / float64(phase.TotalResponses)
	}
	achieved := 0.0
	if cfg.Duration > 0 {
		achieved = float64(phase.TotalResponses) / cfg.Duration.Seconds()
	}
	successRPS := 0.0
	if cfg.Duration > 0 {
		successRPS = float64(phase.Status2xx) / cfg.Duration.Seconds()
	}

	result.Stats["latency_ms"] = latSummary
	if errLatSummary.Count > 0 {
		result.Stats["latency_ms_error"] = errLatSummary
	}
	result.Stats["status_codes"] = phase.StatusCounts
	result.Stats["achieved_rps"] = achieved
	result.Stats["success_rps"] = successRPS
	result.Stats["responses_total"] = phase.TotalResponses
	result.Stats["responses_2xx"] = phase.Status2xx
	result.Stats["rate_2xx"] = rate2xx
	result.Stats["dropped_jobs"] = phase.DroppedJobs
	result.Stats["attempted_jobs"] = phase.AttemptedJobs
	result.Stats["started_jobs"] = phase.AttemptedJobs - phase.DroppedJobs
	result.Stats["net_errors"] = phase.NetErrors
	if phase.NetErrorKinds != nil {
		result.Stats["net_error_kinds"] = phase.NetErrorKinds
	}

	pass := latSummary.Count > 0 &&
		latSummary.P99Ms > 0 &&
		latSummary.P99Ms < cfg.Threshold.P99Ms &&
		rate2xx >= cfg.Threshold.Min2xxRate

	result.Pass = pass
	return result
}
