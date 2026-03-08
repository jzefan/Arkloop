package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newOTPRedisClient(t *testing.T, addr string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: addr, ContextTimeoutEnabled: true})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	return client
}

func TestRedisEmailOTPRiskControlAllowSend(t *testing.T) {
	mr := miniredis.RunT(t)
	client := newOTPRedisClient(t, mr.Addr())
	control := NewRedisEmailOTPRiskControl(client)
	ctx := context.Background()

	for attempt := 1; attempt <= emailOTPSendLimit; attempt++ {
		if err := control.AllowSend(ctx, "Alice@Example.com "); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
		}
	}

	if err := control.AllowSend(ctx, "alice@example.com"); err == nil {
		t.Fatal("expected rate limit error")
	} else if _, ok := err.(OTPRateLimitedError); !ok {
		t.Fatalf("expected OTPRateLimitedError, got %T: %v", err, err)
	}

	if ttl := mr.TTL(emailOTPSendKey("alice@example.com")); ttl <= 0 || ttl > emailOTPSendWindow {
		t.Fatalf("unexpected send ttl: %v", ttl)
	}
}

func TestRedisEmailOTPRiskControlVerifyLockAndReset(t *testing.T) {
	mr := miniredis.RunT(t)
	client := newOTPRedisClient(t, mr.Addr())
	control := NewRedisEmailOTPRiskControl(client)
	ctx := context.Background()

	for attempt := 1; attempt <= emailOTPVerifyFailLimit; attempt++ {
		if err := control.RecordVerifyFailure(ctx, "bob@example.com"); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
		}
	}

	if err := control.EnsureVerifyAllowed(ctx, "bob@example.com"); err == nil {
		t.Fatal("expected lock error")
	} else if _, ok := err.(OTPLockedError); !ok {
		t.Fatalf("expected OTPLockedError, got %T: %v", err, err)
	}

	mr.FastForward(emailOTPVerifyLockDuration + time.Second)
	if err := control.EnsureVerifyAllowed(ctx, "bob@example.com"); err != nil {
		t.Fatalf("expected lock to expire, got: %v", err)
	}

	if err := control.RecordVerifyFailure(ctx, "bob@example.com"); err != nil {
		t.Fatalf("re-record failure: %v", err)
	}
	if err := control.ResetVerifyState(ctx, "bob@example.com"); err != nil {
		t.Fatalf("reset state: %v", err)
	}
	if err := control.EnsureVerifyAllowed(ctx, "bob@example.com"); err != nil {
		t.Fatalf("expected verify allowed after reset, got: %v", err)
	}
}

func TestRedisEmailOTPRiskControlUnavailableWithoutRedis(t *testing.T) {
	control := NewRedisEmailOTPRiskControl(nil)
	ctx := context.Background()

	assertUnavailable := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
		if _, ok := err.(OTPProtectionUnavailableError); !ok {
			t.Fatalf("%s: expected OTPProtectionUnavailableError, got %T: %v", name, err, err)
		}
	}

	assertUnavailable("allow send", control.AllowSend(ctx, "test@example.com"))
	assertUnavailable("ensure verify", control.EnsureVerifyAllowed(ctx, "test@example.com"))
	assertUnavailable("record verify failure", control.RecordVerifyFailure(ctx, "test@example.com"))
	assertUnavailable("reset verify", control.ResetVerifyState(ctx, "test@example.com"))
}
