//go:build desktop

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	api "arkloop/services/api"
	"arkloop/services/shared/desktop"
	worker "arkloop/services/worker"
)

func main() {
	if err := run(); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Worker 先启动：创建 ChannelJobQueue 并注册到 shared/desktop，
	// API 通过 jobEnqueueNotify 钩子将新作业转发到该队列。
	workerErr := make(chan error, 1)
	go func() {
		workerErr <- worker.StartDesktop(ctx)
	}()

	// 等待 Worker 完成共享资源初始化
	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	if err := desktop.WaitReady(waitCtx); err != nil {
		return fmt.Errorf("worker init: %w", err)
	}

	apiErr := make(chan error, 1)
	go func() {
		apiErr <- api.StartDesktop(ctx)
	}()

	select {
	case err := <-apiErr:
		if err != nil {
			fmt.Fprintf(os.Stderr, "api: %v\n", err)
		}
	case err := <-workerErr:
		if err != nil {
			fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		}
	case <-ctx.Done():
	}

	// 触发两侧关闭
	stop()

	// 短暂等待另一侧退出
	graceful := time.After(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-apiErr:
		case <-workerErr:
		case <-graceful:
			return nil
		}
	}
	return nil
}
