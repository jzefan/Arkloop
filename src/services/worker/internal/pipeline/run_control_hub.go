package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"arkloop/services/shared/pgnotify"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type runControlSubscription struct {
	cancelCh chan struct{}
	inputCh  chan struct{}
}

// RunControlHub 通过单连接 LISTEN/NOTIFY 接收取消/输入信号，
// 并在进程内按 runID 分发给对应的 run。
type RunControlHub struct {
	mu   sync.Mutex
	subs map[uuid.UUID]map[*runControlSubscription]struct{}

	running bool
}

func NewRunControlHub() *RunControlHub {
	return &RunControlHub{
		subs: map[uuid.UUID]map[*runControlSubscription]struct{}{},
	}
}

// Start 启动后台 LISTEN 循环。ctx 取消后退出。
func (h *RunControlHub) Start(ctx context.Context, pool *pgxpool.Pool) {
	if h == nil || pool == nil {
		return
	}

	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	go h.loop(ctx, pool)
}

// Subscribe 订阅指定 run 的控制信号。返回的 channel 不会被 close，
// 调用方必须执行 unsubscribe 以释放 map 引用。
func (h *RunControlHub) Subscribe(runID uuid.UUID) (<-chan struct{}, <-chan struct{}, func()) {
	if h == nil {
		return nil, nil, func() {}
	}

	sub := &runControlSubscription{
		cancelCh: make(chan struct{}, 1),
		inputCh:  make(chan struct{}, 1),
	}

	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[uuid.UUID]map[*runControlSubscription]struct{}{}
	}
	if h.subs[runID] == nil {
		h.subs[runID] = map[*runControlSubscription]struct{}{}
	}
	h.subs[runID][sub] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			m := h.subs[runID]
			delete(m, sub)
			if len(m) == 0 {
				delete(h.subs, runID)
			}
		})
	}
	return sub.cancelCh, sub.inputCh, unsubscribe
}

func (h *RunControlHub) loop(ctx context.Context, pool *pgxpool.Pool) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 30 * time.Second
	)
	delay := baseDelay

	for {
		if ctx.Err() != nil {
			return
		}

		err := h.listenOnce(ctx, pool)
		if ctx.Err() != nil {
			return
		}

		slog.WarnContext(ctx, "run control hub: LISTEN connection lost, retrying", "err", err, "delay", delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func (h *RunControlHub) listenOnce(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `LISTEN "`+pgnotify.ChannelRunCancel+`"`); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `LISTEN "`+pgnotify.ChannelRunInput+`"`); err != nil {
		return err
	}

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		h.dispatch(n.Channel, n.Payload)
	}
}

func (h *RunControlHub) dispatch(channel string, payload string) {
	cleaned := strings.TrimSpace(payload)
	runID, err := uuid.Parse(cleaned)
	if err != nil {
		return
	}

	h.mu.Lock()
	m := h.subs[runID]
	targets := make([]*runControlSubscription, 0, len(m))
	for sub := range m {
		targets = append(targets, sub)
	}
	h.mu.Unlock()

	switch channel {
	case pgnotify.ChannelRunCancel:
		for _, sub := range targets {
			select {
			case sub.cancelCh <- struct{}{}:
			default:
			}
		}
	case pgnotify.ChannelRunInput:
		for _, sub := range targets {
			select {
			case sub.inputCh <- struct{}{}:
			default:
			}
		}
	}
}
