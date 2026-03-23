package rollout

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"arkloop/services/shared/objectstore"

	"github.com/google/uuid"
)

// Recorder 将 RolloutItem 异步写入 S3（JSONL 格式）。
// 线程安全，通过 buffered channel + flush goroutine 实现。
type Recorder struct {
	store   objectstore.BlobStore
	runID   uuid.UUID
	key     string // S3 object key: "run/{runID}.jsonl"
	buf     chan RolloutItem
	closed  chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	flushed bool
}

const recorderBufSize = 64 // channel buffer size

func NewRecorder(store objectstore.BlobStore, runID uuid.UUID) *Recorder {
	return &Recorder{
		store:  store,
		runID:  runID,
		key:    "run/" + runID.String() + ".jsonl",
		buf:    make(chan RolloutItem, recorderBufSize),
		closed: make(chan struct{}),
	}
}

// Append 将一个 RolloutItem 异步写入 S3。不阻塞调用方。
func (r *Recorder) Append(ctx context.Context, item RolloutItem) error {
	r.mu.Lock()
	flushed := r.flushed
	r.mu.Unlock()
	if flushed {
		return nil
	}
	select {
	case r.buf <- item:
		return nil
	case <-r.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AppendSync 同步写入（用于 run_end 等必须确认的条目）。
func (r *Recorder) AppendSync(ctx context.Context, item RolloutItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// 追加模式：读取现有内容，拼接，新写回
	existing, err := r.store.Get(ctx, r.key)
	if err != nil && !objectstore.IsNotFound(err) {
		return err
	}
	combined := append(existing, data...)
	return r.store.Put(ctx, r.key, combined)
}

// Start 启动后台 flush goroutine。defer Recorder.Close() 调用。
func (r *Recorder) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.flushLoop(ctx)
}

// Close 等待所有缓冲数据写入 S3，然后关闭。
func (r *Recorder) Close(ctx context.Context) error {
	close(r.closed)
	r.wg.Wait()
	return nil
}

func (r *Recorder) flushLoop(ctx context.Context) {
	defer r.wg.Done()
	var batch []RolloutItem
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.closed:
			// 最后的 flush
			r.flushBatch(ctx, batch)
			return
		case <-ticker.C:
			r.flushBatch(ctx, batch)
			batch = nil
		case item := <-r.buf:
			batch = append(batch, item)
			if len(batch) >= recorderBufSize/2 {
				r.flushBatch(ctx, batch)
				batch = nil
			}
		}
	}
}

func (r *Recorder) flushBatch(ctx context.Context, batch []RolloutItem) {
	if len(batch) == 0 {
		return
	}
	var data []byte
	for _, item := range batch {
		enc, err := json.Marshal(item)
		if err != nil {
			continue
		}
		data = append(data, enc...)
		data = append(data, '\n')
	}
	if len(data) == 0 {
		return
	}
	// 读取现有内容，拼接到末尾
	existing, err := r.store.Get(ctx, r.key)
	if err != nil && !objectstore.IsNotFound(err) {
		return
	}
	r.store.Put(ctx, r.key, append(existing, data...))
}
