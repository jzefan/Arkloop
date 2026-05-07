package ratelimit

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestBucket 创建一个带可控时钟的 TokenBucket，用于测试时推进时间。
func newTestBucket(t *testing.T, capacity, ratePerMinute float64) (*TokenBucket, *float64) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := Config{Capacity: capacity, RatePerMinute: ratePerMinute}
	b, err := NewTokenBucket(rdb, cfg)
	if err != nil {
		t.Fatalf("NewTokenBucket: %v", err)
	}

	// 注入可控时钟
	ts := float64(1_700_000_000)
	b.now = func() float64 { return ts }

	return b, &ts
}

func TestTokenBucket_AllowsWithinCapacity(t *testing.T) {
	b, _ := newTestBucket(t, 5, 60)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		result, err := b.Consume(ctx, "test:key")
		if err != nil {
			t.Fatalf("Consume[%d]: %v", i, err)
		}
		if !result.Allowed {
			t.Fatalf("expected allowed on request %d", i)
		}
	}
}

func TestTokenBucket_BlocksWhenExhausted(t *testing.T) {
	b, _ := newTestBucket(t, 3, 60)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := b.Consume(ctx, "test:drain"); err != nil {
			t.Fatalf("Consume[%d]: %v", i, err)
		}
	}

	result, err := b.Consume(ctx, "test:drain")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if result.Allowed {
		t.Fatal("expected request to be blocked after bucket exhaustion")
	}
	if result.RetryAfterSecs <= 0 {
		t.Fatalf("expected positive RetryAfterSecs, got %d", result.RetryAfterSecs)
	}
}

func TestTokenBucket_RefillsOverTime(t *testing.T) {
	// capacity=2, rate=60/min = 1 token/sec; 需要推进 2 秒填满
	b, ts := newTestBucket(t, 2, 60)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := b.Consume(ctx, "test:refill"); err != nil {
			t.Fatalf("Consume[%d]: %v", i, err)
		}
	}

	// 验证已耗尽
	r, _ := b.Consume(ctx, "test:refill")
	if r.Allowed {
		t.Fatal("bucket should be exhausted")
	}

	// 推进 2 秒
	*ts += 2.0

	result, err := b.Consume(ctx, "test:refill")
	if err != nil {
		t.Fatalf("Consume after refill: %v", err)
	}
	if !result.Allowed {
		t.Fatal("expected request to be allowed after time advance")
	}
}

func TestTokenBucket_IsolatesKeys(t *testing.T) {
	b, _ := newTestBucket(t, 1, 60)
	ctx := context.Background()

	if _, err := b.Consume(ctx, "test:a"); err != nil {
		t.Fatalf("Consume a: %v", err)
	}

	result, err := b.Consume(ctx, "test:b")
	if err != nil {
		t.Fatalf("Consume b: %v", err)
	}
	if !result.Allowed {
		t.Fatal("key B should not be affected by key A exhaustion")
	}
}

func TestNewTokenBucket_ValidationErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	if _, err := NewTokenBucket(nil, Config{Capacity: 10, RatePerMinute: 60}); err == nil {
		t.Error("expected error for nil redis client")
	}
	if _, err := NewTokenBucket(rdb, Config{Capacity: 0, RatePerMinute: 60}); err == nil {
		t.Error("expected error for zero capacity")
	}
	if _, err := NewTokenBucket(rdb, Config{Capacity: 10, RatePerMinute: 0}); err == nil {
		t.Error("expected error for zero rate")
	}
}
