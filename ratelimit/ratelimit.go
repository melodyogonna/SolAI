// Package ratelimit provides rate limiting and retry strategies for tools and agents.
package ratelimit

import "context"

// RateLimitStrategy controls how many operations are allowed over time.
// Implementations block until a slot is available or the context is cancelled.
type RateLimitStrategy interface {
	// Wait blocks until the caller is allowed to proceed.
	// Returns ctx.Err() if the context is cancelled while waiting.
	Wait(ctx context.Context) error
}

// RetryStrategy controls how failed operations are retried.
type RetryStrategy interface {
	// Execute calls fn, retrying on error according to the strategy until fn
	// succeeds, retries are exhausted, or ctx is cancelled.
	// Returns the last error if all attempts fail, or ctx.Err() on cancellation.
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}
