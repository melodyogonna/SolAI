package ratelimit

import (
	"context"
	"sync"
	"time"
)

// FixedWindowLimiter allows at most Limit calls per Window duration.
// Once the limit is reached, Wait blocks until the current window expires.
type FixedWindowLimiter struct {
	limit       int
	window      time.Duration
	mu          sync.Mutex
	count       int
	windowStart time.Time
}

// NewFixedWindowLimiter returns a limiter that allows at most limit calls
// within each window duration.
func NewFixedWindowLimiter(limit int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		limit:       limit,
		window:      window,
		windowStart: time.Now(),
	}
}

// Wait blocks until the caller is allowed to proceed within the current window.
// If the window has expired it resets the counter and allows immediately.
// Returns ctx.Err() if the context is cancelled while waiting.
func (f *FixedWindowLimiter) Wait(ctx context.Context) error {
	for {
		f.mu.Lock()
		now := time.Now()
		if now.Sub(f.windowStart) >= f.window {
			f.windowStart = now
			f.count = 0
		}
		if f.count < f.limit {
			f.count++
			f.mu.Unlock()
			return nil
		}
		resetAt := f.windowStart.Add(f.window)
		f.mu.Unlock()

		waitDur := time.Until(resetAt)
		if waitDur <= 0 {
			// Window has already passed; loop to reset.
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDur):
			// Window expired; loop to re-check.
		}
	}
}
