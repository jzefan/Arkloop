package main

import (
	"context"
	"errors"
	"testing"

	"arkloop/services/sandbox/internal/app"
)

type fakeLifecycleStore struct {
	days int
	err  error
	set  bool
}

func (s *fakeLifecycleStore) SetLifecycleExpirationDays(_ context.Context, days int) error {
	s.set = true
	s.days = days
	return s.err
}

func TestApplyStateStoreLifecycle(t *testing.T) {
	store := &fakeLifecycleStore{}
	cfg := app.DefaultConfig()
	cfg.SessionStateTTLDays = 7

	if err := applyStateStoreLifecycle(context.Background(), cfg, store); err != nil {
		t.Fatalf("apply lifecycle failed: %v", err)
	}
	if !store.set || store.days != 7 {
		t.Fatalf("unexpected lifecycle call: %#v", store)
	}
}

func TestApplyStateStoreLifecycleSkipWhenDisabled(t *testing.T) {
	store := &fakeLifecycleStore{}
	cfg := app.DefaultConfig()
	cfg.SessionStateTTLDays = 0

	if err := applyStateStoreLifecycle(context.Background(), cfg, store); err != nil {
		t.Fatalf("apply lifecycle failed: %v", err)
	}
	if store.set {
		t.Fatalf("lifecycle should be skipped, got %#v", store)
	}
}

func TestApplyStateStoreLifecycleReturnsError(t *testing.T) {
	store := &fakeLifecycleStore{err: errors.New("boom")}
	cfg := app.DefaultConfig()
	cfg.SessionStateTTLDays = 3

	if err := applyStateStoreLifecycle(context.Background(), cfg, store); err == nil {
		t.Fatal("expected lifecycle error")
	}
}
