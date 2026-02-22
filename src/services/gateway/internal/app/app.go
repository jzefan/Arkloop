package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"arkloop/services/gateway/internal/ipfilter"
	"arkloop/services/gateway/internal/proxy"
	"arkloop/services/gateway/internal/ratelimit"
	sharedredis "arkloop/services/shared/redis"

	goredis "github.com/redis/go-redis/v9"
)

type Application struct {
	config Config
	logger *JSONLogger
}

func NewApplication(config Config, logger *JSONLogger) (*Application, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	return &Application{config: config, logger: logger}, nil
}

func (a *Application) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	p, err := proxy.New(proxy.Config{Upstream: a.config.Upstream})
	if err != nil {
		return fmt.Errorf("proxy: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.Handle("/", p)

	var (
		limiter    ratelimit.Limiter
		ipFilter   *ipfilter.Filter
		rdb        *goredis.Client
	)
	if strings.TrimSpace(a.config.RedisURL) != "" {
		var redisErr error
		rdb, redisErr = sharedredis.NewClient(ctx, a.config.RedisURL)
		if redisErr != nil {
			return fmt.Errorf("redis: %w", redisErr)
		}
		defer rdb.Close()

		bucket, err := ratelimit.NewTokenBucket(rdb, a.config.RateLimit)
		if err != nil {
			return fmt.Errorf("ratelimit: %w", err)
		}
		limiter = bucket
		ipFilter = ipfilter.NewFilter(rdb)

		a.logger.Info("ratelimit enabled", LogFields{}, map[string]any{
			"capacity":        a.config.RateLimit.Capacity,
			"rate_per_minute": a.config.RateLimit.RatePerMinute,
		})
		a.logger.Info("ipfilter enabled", LogFields{}, nil)
	}

	inner := http.Handler(mux)
	if limiter != nil {
		inner = ratelimit.NewRateLimitMiddleware(inner, limiter, a.config.JWTSecret, rdb)
	}
	if ipFilter != nil {
		inner = ipFilter.Middleware(inner)
	}
	handler := recoverMiddleware(traceMiddleware(inner, a.logger), a.logger)

	listener, err := net.Listen("tcp", a.config.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	a.logger.Info("gateway started", LogFields{}, map[string]any{
		"addr":     a.config.Addr,
		"upstream": a.config.Upstream,
	})

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}

	err = <-errCh
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"code":"http.method_not_allowed","message":"Method Not Allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	payload, _ := json.Marshal(map[string]string{"status": "ok"})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
