//go:build desktop

package worker

import (
	"context"

	"arkloop/services/worker/internal/desktoprun"
)

// StartDesktop 启动桌面模式 Worker 消费循环。阻塞直到 ctx 取消或出错。
func StartDesktop(ctx context.Context) error {
	return desktoprun.RunDesktop(ctx)
}
