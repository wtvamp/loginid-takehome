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
// caller from this cap.
//
// Reserve/Settle, not a bare check-then-record: an earlier shape split
// this into Allowed() (check) and RecordTouches() (record), with the
// DAO call — unlocked, and potentially slow — happening in between.
// Under concurrent requests from the same sub, every one of them could
// read Allowed()==true before any of them recorded a touch, letting N
// concurrent requests each add up to a full page's worth of distinct
// touches — overshooting the cap by a multiple of the page size, not the
// one-request overshoot the design accepts (Nolan Reyes, PR #26 review).
// Reserve atomically checks capacity AND books a worst-case-sized
// placeholder for the request in progress, so a second concurrent
// request sees the first's reservation and is correctly denied before
// either request's DAO call returns.
type TouchCounter interface {
	// Reserve reports whether sub may proceed with a request that could
	// touch up to maxNewTouches additional distinct records — false once
	// sub's committed-plus-reserved count already meets the cap. A true
	// result books maxNewTouches against the window immediately; the
	// caller must call Settle exactly once per successful Reserve
	// (whether or not the request that follows succeeds) to release the
	// reservation and record the actual distinct ids touched.
	Reserve(ctx context.Context, sub string, maxNewTouches int) (bool, error)
	// Settle releases a reservation of size reserved (the same value
	// passed to the Reserve call this settles) and records recordIDs
	// (already deduplicated by the caller) as actually touched — fewer
	// than reserved is the common case (a page returned fewer rows than
	// its cap, or the request failed and recordIDs is empty).
	Settle(ctx context.Context, sub string, recordIDs []string, reserved int) error
}

// touchWindow is one caller's rolling-24h bookkeeping. reserved is the
// sum of in-flight requests' booked-but-not-yet-settled budget; a
// request is denied once len(touched)+reserved reaches the cap, not just
// once len(touched) does.
type touchWindow struct {
	windowStart time.Time
	touched     map[string]struct{}
	reserved    int
}

// InProcessTouchCounter is an in-process implementation — Redis or any
// other shared store is a drop-in replacement behind the same interface
// for a multi-instance deployment; the mechanism is this track's call
// (handoff-03-auth.md v3, explicitly).
//
// windows has no active eviction beyond lazy replacement-on-next-access
// (currentWindow swaps in a fresh entry once a sub's window has rolled
// over) — a sub that stops calling entirely leaves its entry in the map
// forever. sweep(), called opportunistically from Reserve, bounds this:
// harmless if sub cardinality is small and fixed (registered client
// credentials), a real concern if sub is minted per end-user session
// (Nolan Reyes, PR #26 review) — worth confirming what populates sub
// before this backs a real deployment.
type InProcessTouchCounter struct {
	mu        sync.Mutex
	windows   map[string]*touchWindow
	cap       int
	window    time.Duration
	now       func() time.Time
	lastSweep time.Time
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

// sweepLocked drops any window that expired at least one full window
// duration ago — called with c.mu already held, at most once per window
// duration, so it doesn't add per-request cost. A generous margin (only
// sweeping windows that are stale, not merely expired) avoids racing a
// window that just rolled over and is mid-replacement in currentWindow.
func (c *InProcessTouchCounter) sweepLocked(now time.Time) {
	if now.Sub(c.lastSweep) < c.window {
		return
	}
	c.lastSweep = now
	for sub, w := range c.windows {
		if now.Sub(w.windowStart) >= 2*c.window {
			delete(c.windows, sub)
		}
	}
}

func (c *InProcessTouchCounter) Reserve(_ context.Context, sub string, maxNewTouches int) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.sweepLocked(now)
	w := c.currentWindow(sub, now)
	if len(w.touched)+w.reserved >= c.cap {
		return false, nil
	}
	w.reserved += maxNewTouches
	return true, nil
}

// touchReservation guarantees a booked TouchCounter reservation is
// released exactly once, by a deferred cleanup unless settle() already
// ran — closing the "a manual Settle call at every return path" fragility
// (Nolan Reyes, PR #26 review): the next handler that reserves and
// forgets one branch's Settle call silently burns that caller's budget
// for the rest of the window, and nothing catches it, including a panic
// unwinding past a manual call. A defer can't be forgotten the same way.
type touchReservation struct {
	tc       TouchCounter
	sub      string
	reserved int
	done     bool
}

// reserveTouches books maxNewTouches against sub's window if the cap
// allows it. ok is false if the cap is already met — the caller must not
// use the returned *touchReservation in that case (it is nil).
func reserveTouches(ctx context.Context, tc TouchCounter, sub string, maxNewTouches int) (tr *touchReservation, ok bool, err error) {
	ok, err = tc.Reserve(ctx, sub, maxNewTouches)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &touchReservation{tc: tc, sub: sub, reserved: maxNewTouches}, true, nil
}

// release is a no-op if settle already ran; call it via `defer
// tr.release(ctx)` immediately after a successful reserveTouches, so
// every return path — including one added later without updating this
// function, and a panic unwind — releases an unsettled reservation.
// Detaches ctx from the triggering request's own cancellation
// (context.WithoutCancel): this is a release-shaped operation exactly
// like the DAO credential-write detach points decisions/
// context-propagation.md already rules on, and the most common reason
// this fires at all is that ctx just expired or was canceled upstream —
// handing that same dying context to the cleanup would be handing the
// release the same failure the request itself just hit (Oren Castellan,
// PR #26 review).
func (tr *touchReservation) release(ctx context.Context) {
	if tr == nil || tr.done {
		return
	}
	tr.done = true
	_ = tr.tc.Settle(context.WithoutCancel(ctx), tr.sub, nil, tr.reserved)
}

// settle records the actual distinct record ids touched and marks the
// reservation finalized, so the deferred release becomes a no-op.
func (tr *touchReservation) settle(ctx context.Context, recordIDs []string) error {
	if tr == nil || tr.done {
		return nil
	}
	tr.done = true
	return tr.tc.Settle(ctx, tr.sub, recordIDs, tr.reserved)
}

func (c *InProcessTouchCounter) Settle(_ context.Context, sub string, recordIDs []string, reserved int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Deliberately does NOT call currentWindow: if the window rolled over
	// between this sub's Reserve and this Settle, the reservation being
	// released belonged to a window that's already been replaced (or is
	// about to be) — there is nothing correct to settle it against, and
	// creating a new window here would record this request's touches
	// against a window it was never capped against. The rare in-flight
	// request that straddles a window boundary loses its reservation
	// bookkeeping harmlessly rather than corrupting the new window's count.
	w, ok := c.windows[sub]
	if !ok {
		return nil
	}
	for _, id := range recordIDs {
		w.touched[id] = struct{}{}
	}
	w.reserved -= reserved
	if w.reserved < 0 {
		w.reserved = 0
	}
	return nil
}
