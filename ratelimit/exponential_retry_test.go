package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errTemp = errors.New("temporary error")

func TestExponentialRetry_SucceedsOnFirstAttempt(t *testing.T) {
	r := NewExponentialRetry(3, 10*time.Millisecond, time.Second, 2)
	calls := 0
	err := r.Execute(context.Background(), func(ctx context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestExponentialRetry_RetriesAndSucceeds(t *testing.T) {
	r := NewExponentialRetry(5, 10*time.Millisecond, time.Second, 2)
	calls := 0
	err := r.Execute(context.Background(), func(ctx context.Context) error {
		calls++
		if calls < 3 {
			return errTemp
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestExponentialRetry_ExhaustsAttempts(t *testing.T) {
	r := NewExponentialRetry(3, 10*time.Millisecond, time.Second, 2)
	calls := 0
	err := r.Execute(context.Background(), func(ctx context.Context) error {
		calls++
		return errTemp
	})
	if !errors.Is(err, errTemp) {
		t.Fatalf("expected errTemp, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestExponentialRetry_ContextCancelled(t *testing.T) {
	r := NewExponentialRetry(10, 200*time.Millisecond, time.Second, 2)
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := r.Execute(ctx, func(ctx context.Context) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return errTemp
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call before cancel, got %d", calls)
	}
}

func TestExponentialRetry_DelayGrowsExponentially(t *testing.T) {
	r := NewExponentialRetry(4, 50*time.Millisecond, time.Second, 2)
	start := time.Now()
	_ = r.Execute(context.Background(), func(ctx context.Context) error {
		return errTemp
	})
	// 3 waits: 50ms, 100ms, 200ms = 350ms total minimum
	elapsed := time.Since(start)
	if elapsed < 300*time.Millisecond {
		t.Errorf("expected at least 300ms of total delay, got %v", elapsed)
	}
}

func TestExponentialRetry_DelayCapppedAtMax(t *testing.T) {
	r := NewExponentialRetry(4, 50*time.Millisecond, 60*time.Millisecond, 10)
	start := time.Now()
	_ = r.Execute(context.Background(), func(ctx context.Context) error {
		return errTemp
	})
	// 3 waits each capped at 60ms = 180ms; with multiplier=10 uncapped would be 50+500+5000ms
	elapsed := time.Since(start)
	if elapsed > 400*time.Millisecond {
		t.Errorf("delay was not capped: elapsed %v", elapsed)
	}
}
