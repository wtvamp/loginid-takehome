package api

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces handoff-03-auth.md v3's per-client-credential
// rate limits (60 req/min for read:own/read:any, 10 req/min for
// profile:search), keyed per sub+scope. The mechanism is this track's
// call (v3, explicitly) — an interface so handler tests use a fake
// instead of real wall-clock behavior.
type RateLimiter interface {
	// Allow reports whether sub may make one more request under scope
	// right now, given limit requests per rolling minute.
	Allow(ctx context.Context, sub string, scope Scope, limit int) (bool, error)
}

// InProcessRateLimiter is a fixed-window (per-wall-clock-minute) limiter,
// in-process rather than Redis-backed — the simplest mechanism satisfying
// v3's "the mechanism is your call" framing, sufficient for this
// take-home's single-instance deployment. A fixed window (vs. sliding) can
// admit up to 2x limit across a window boundary; accepted here as a
// deliberate simplification, not hidden — a production multi-instance
// deployment would need a shared store (Redis) instead.
type InProcessRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	windowStart time.Time
	count       int
}

// NewInProcessRateLimiter constructs a ready-to-use limiter.
func NewInProcessRateLimiter() *InProcessRateLimiter {
	return &InProcessRateLimiter{
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

func (r *InProcessRateLimiter) Allow(_ context.Context, sub string, scope Scope, limit int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := sub + "|" + string(scope)
	now := r.now()
	b, ok := r.buckets[key]
	if !ok || now.Sub(b.windowStart) >= time.Minute {
		b = &bucket{windowStart: now, count: 0}
		r.buckets[key] = b
	}
	if b.count >= limit {
		return false, nil
	}
	b.count++
	return true, nil
}
