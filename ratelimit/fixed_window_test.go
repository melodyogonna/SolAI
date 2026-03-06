package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestFixedWindowLimiter_AllowsUpToLimit(t *testing.T) {
	limiter := NewFixedWindowLimiter(3, time.Second)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := limiter.Wait(ctx); err != nil {
			t.Fatalf("attempt %d: unexpected error: %v", i+1, err)
		}
	}
}

func TestFixedWindowLimiter_BlocksAfterLimit(t *testing.T) {
	limiter := NewFixedWindowLimiter(1, 200*time.Millisecond)
	ctx := context.Background()

	if err := limiter.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	// Second call should block until the window resets (~200ms), not fail instantly.
	start := time.Now()
	if err := limiter.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected Wait to block for ~200ms, returned after %v", elapsed)
	}
}

func TestFixedWindowLimiter_ContextCancelled(t *testing.T) {
	limiter := NewFixedWindowLimiter(1, 10*time.Second)
	ctx := context.Background()

	// Exhaust the limit.
	if err := limiter.Wait(ctx); err != nil {
		t.Fatal(err)
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := limiter.Wait(cancelCtx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFixedWindowLimiter_ResetsAfterWindow(t *testing.T) {
	limiter := NewFixedWindowLimiter(2, 100*time.Millisecond)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := limiter.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(150 * time.Millisecond)

	// Window should have reset — these two calls must succeed without blocking.
	for i := 0; i < 2; i++ {
		start := time.Now()
		if err := limiter.Wait(ctx); err != nil {
			t.Fatal(err)
		}
		if time.Since(start) > 50*time.Millisecond {
			t.Errorf("call %d after reset took too long: %v", i+1, time.Since(start))
		}
	}
}

func TestFixedWindowLimiter_ConcurrentSafe(t *testing.T) {
	const limit = 10
	limiter := NewFixedWindowLimiter(limit, time.Second)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Wait(ctx); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if limiter.count != limit {
		t.Errorf("expected count %d, got %d", limit, limiter.count)
	}
}
