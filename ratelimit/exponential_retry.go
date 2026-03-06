package ratelimit

import (
	"context"
	"time"
)

// ExponentialRetry retries a function with exponentially increasing delays
// between attempts, capped at a maximum delay.
type ExponentialRetry struct {
	maxAttempts  int
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
}

// NewExponentialRetry returns a retry strategy that attempts fn up to
// maxAttempts times. The delay between attempts starts at initialDelay and
// is multiplied by multiplier after each failure, capped at maxDelay.
func NewExponentialRetry(maxAttempts int, initialDelay, maxDelay time.Duration, multiplier float64) *ExponentialRetry {
	return &ExponentialRetry{
		maxAttempts:  maxAttempts,
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
		multiplier:   multiplier,
	}
}

// Execute calls fn up to maxAttempts times. On each failure it waits an
// exponentially increasing duration before the next attempt.
// Returns nil on the first success, ctx.Err() if the context is cancelled,
// or the last error once all attempts are exhausted.
func (e *ExponentialRetry) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	delay := e.initialDelay
	var lastErr error

	for attempt := 0; attempt < e.maxAttempts; attempt++ {
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if attempt == e.maxAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		next := time.Duration(float64(delay) * e.multiplier)
		if next > e.maxDelay {
			next = e.maxDelay
		}
		delay = next
	}
	return lastErr
}
