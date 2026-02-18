package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"arkloop/services/api_go/internal/data"
	apihttp "arkloop/services/api_go/internal/http"
	"arkloop/services/api_go/internal/observability"
)

type Application struct {
	config Config
	logger *observability.JSONLogger
}

func NewApplication(config Config, logger *observability.JSONLogger) (*Application, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, fmt.Errorf("logger 不能为空")
	}
	return &Application{
		config: config,
		logger: logger,
	}, nil
}

func (a *Application) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var schemaRepo *data.SchemaRepository
	var poolCloser func()

	dsn := strings.TrimSpace(a.config.DatabaseDSN)
	if dsn != "" {
		pool, err := data.NewPool(ctx, dsn)
		if err != nil {
			return err
		}
		poolCloser = pool.Close

		repo, err := data.NewSchemaRepository(pool)
		if err != nil {
			pool.Close()
			return err
		}
		schemaRepo = repo
	}
	if poolCloser != nil {
		defer poolCloser()
	}

	listener, err := net.Listen("tcp", a.config.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()

	server := &http.Server{
		Handler: apihttp.NewHandler(apihttp.HandlerConfig{
			Logger:               a.logger,
			TrustIncomingTraceID: a.config.TrustIncomingTraceID,
			SchemaRepository:     schemaRepo,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()

	a.logger.Info("api_go 已启动", observability.LogFields{}, map[string]any{"addr": listener.Addr().String()})

	select {
	case <-ctx.Done():
		a.logger.Info("api_go 收到停止信号", observability.LogFields{}, map[string]any{"reason": ctx.Err().Error()})
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
