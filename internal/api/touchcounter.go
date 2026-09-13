package api

import (
	"context"
	"sync"
	"time"
)

// TouchCounter enforces handoff-03-auth.md v3's cumulative
// distinct-record-touch cap per caller per rolling 24h window — the
// control that actually resists decomposition-style enumeration, tracked
// independently of the per-minute RateLimiter above. Applies to both
// ScopeSearch and ScopeReadAny unconditionally: a valid ReasonCode
// satisfies the per-call Authorize() policy check but does not exempt the
// caller from this cap (the exact regression handoff-03-auth.md v3 fixed
// — an earlier wording made this counter a no-op for read:any, since that
// scope requires a ReasonCode on every call by design).
type TouchCounter interface {
	// Allowed reports whether sub may make one more request under the
	// counted scopes right now — false once sub has already crossed the
	// cap within the current rolling window. Checked before the request
	// is served, per v3's hard-deny ruling: crossing the cap denies every
	// subsequent request for the remainder of the window, in addition to
	// alerting, not instead of it.
	Allowed(ctx context.Context, sub string) (bool, error)
	// RecordTouches adds recordIDs (already deduplicated by the caller)
	// to sub's distinct-touch set for the current rolling window and
	// returns the count after recording. Called once per request, with
	// the record ids about to be returned — after the request has
	// already been allowed to proceed, so a request that pushes the
	// count over the cap still completes; only the next request is
	// denied.
	RecordTouches(ctx context.Context, sub string, recordIDs []string) (count int, err error)
}

// touchWindow is one caller's rolling-24h bookkeeping.
type touchWindow struct {
	windowStart time.Time
	touched     map[string]struct{}
}

// InProcessTouchCounter is an in-process implementation — Redis or any
// other shared store is a drop-in replacement behind the same interface
// for a multi-instance deployment; the mechanism is this track's call
// (handoff-03-auth.md v3, explicitly).
type InProcessTouchCounter struct {
	mu      sync.Mutex
	windows map[string]*touchWindow
	cap     int
	window  time.Duration
	now     func() time.Time
}

// NewInProcessTouchCounter constructs a counter enforcing capPerWindow
// distinct record touches per rolling window (24h in production; a
// shorter window is accepted for tests).
func NewInProcessTouchCounter(capPerWindow int, window time.Duration) *InProcessTouchCounter {
	return &InProcessTouchCounter{
		windows: make(map[string]*touchWindow),
		cap:     capPerWindow,
		window:  window,
		now:     time.Now,
	}
}

func (c *InProcessTouchCounter) currentWindow(sub string, now time.Time) *touchWindow {
	w, ok := c.windows[sub]
	if !ok || now.Sub(w.windowStart) >= c.window {
		w = &touchWindow{windowStart: now, touched: make(map[string]struct{})}
		c.windows[sub] = w
	}
	return w
}

func (c *InProcessTouchCounter) Allowed(_ context.Context, sub string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.currentWindow(sub, c.now())
	return len(w.touched) < c.cap, nil
}

func (c *InProcessTouchCounter) RecordTouches(_ context.Context, sub string, recordIDs []string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.currentWindow(sub, c.now())
	for _, id := range recordIDs {
		w.touched[id] = struct{}{}
	}
	return len(w.touched), nil
}
