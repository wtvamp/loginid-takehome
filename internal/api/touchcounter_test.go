package api

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInProcessTouchCounter_CountsDistinctRecordsOnly(t *testing.T) {
	c := NewInProcessTouchCounter(3, 24*time.Hour)
	ctx := context.Background()

	if _, err := c.Reserve(ctx, "clientA", 3); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := c.Settle(ctx, "clientA", []string{"r1", "r2", "r1"}, 3); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// A 4th distinct touch should now be denied: r1+r2 = 2 distinct, cap
	// is 3, so exactly one more distinct id fits.
	allowed, err := c.Reserve(ctx, "clientA", 2)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !allowed {
		t.Fatal("should still have budget for 1 more distinct touch (2 used of 3)")
	}
	if err := c.Settle(ctx, "clientA", []string{"r3", "r4"}, 2); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// Duplicates (r1, r2) must not have inflated the count: after settling
	// [r1,r2,r1] then [r3,r4], the distinct set is {r1,r2,r3,r4} = 4,
	// which is already over the cap of 3 — the next Reserve must deny.
	allowed, err = c.Reserve(ctx, "clientA", 1)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if allowed {
		t.Error("should be denied once distinct touches reach the cap")
	}
}

// TestInProcessTouchCounter_ReserveBooksBudgetBeforeSettle is the core
// concurrency fix (Nolan Reyes, PR #26 review): a second Reserve must see
// the first's booked budget even before the first has called Settle —
// otherwise concurrent requests can each pass Reserve and together
// overshoot the cap by a multiple of their own page size.
func TestInProcessTouchCounter_ReserveBooksBudgetBeforeSettle(t *testing.T) {
	c := NewInProcessTouchCounter(10, 24*time.Hour)
	ctx := context.Background()

	// First request reserves its full page size (10) — books the whole
	// cap immediately, before it has queried anything or called Settle.
	ok1, err := c.Reserve(ctx, "clientA", 10)
	if err != nil || !ok1 {
		t.Fatalf("first Reserve: ok=%v err=%v, want true/nil", ok1, err)
	}

	// A second concurrent request's Reserve must now be denied — not
	// because any touches were recorded yet, but because the first
	// request's reservation already consumes the whole window's budget.
	ok2, err := c.Reserve(ctx, "clientB2", 10) // different sub: must NOT be denied
	if err != nil || !ok2 {
		t.Fatalf("a different sub's Reserve must not be affected by clientA's reservation: ok=%v err=%v", ok2, err)
	}
	ok3, err := c.Reserve(ctx, "clientA", 1)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if ok3 {
		t.Error("a second concurrent Reserve for the SAME sub must be denied while the first reservation is still outstanding")
	}
}

// TestInProcessTouchCounter_ConcurrentReservesNeverExceedCap fires real
// goroutines — not a sequential simulation — at the same sub, each
// reserving a full page's worth of budget, and asserts the total ever
// granted never exceeds the cap. This is the actual failure mode Nolan
// Reyes's review described: N concurrent requests each reading
// "allowed" before any of them recorded a touch. Run with -race.
func TestInProcessTouchCounter_ConcurrentReservesNeverExceedCap(t *testing.T) {
	const capLimit = 100
	const pageSize = 50
	const concurrentRequests = 20 // 20*50 = 1000, 10x the cap if the race isn't closed

	c := NewInProcessTouchCounter(capLimit, 24*time.Hour)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := c.Reserve(ctx, "clientA", pageSize)
			if err != nil {
				t.Errorf("Reserve: %v", err)
				return
			}
			if ok {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if granted*pageSize > capLimit {
		t.Errorf("granted %d reservations of size %d = %d total booked, want at most %d (the cap)", granted, pageSize, granted*pageSize, capLimit)
	}
}

// TestInProcessTouchCounter_SettleReleasesUnusedReservation covers the
// common case: a page returns fewer distinct rows than its reserved
// page size, and the difference must be released, not permanently
// consumed.
func TestInProcessTouchCounter_SettleReleasesUnusedReservation(t *testing.T) {
	c := NewInProcessTouchCounter(5, 24*time.Hour)
	ctx := context.Background()

	if _, err := c.Reserve(ctx, "clientA", 5); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	// Only 2 of the reserved 5 were actually touched (a short page, or a
	// denied/failed request settling with recordIDs=nil).
	if err := c.Settle(ctx, "clientA", []string{"r1", "r2"}, 5); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// The unused 3 must be released: a new request should be able to
	// reserve up to 3 more before hitting the cap of 5.
	ok, err := c.Reserve(ctx, "clientA", 3)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !ok {
		t.Error("unused reservation budget must be released by Settle, not held forever")
	}
}

// TestInProcessTouchCounter_HardDeniesAfterCrossingCap is
// handoff-03-auth.md v3's ruling: crossing the cap hard-denies further
// requests for the remainder of the window — the request that crosses
// the cap itself still completes (it already held a valid reservation),
// but the next one is denied.
func TestInProcessTouchCounter_HardDeniesAfterCrossingCap(t *testing.T) {
	c := NewInProcessTouchCounter(2, 24*time.Hour)
	ctx := context.Background()

	ok, err := c.Reserve(ctx, "clientA", 3) // reserving more than the cap is itself allowed once
	if err != nil || !ok {
		t.Fatalf("first Reserve: ok=%v err=%v, want true/nil", ok, err)
	}
	if err := c.Settle(ctx, "clientA", []string{"r1", "r2", "r3"}, 3); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	// This request's own 3 touches pushed the distinct count to 3, over
	// the cap of 2 — but it had already been allowed to proceed.
	allowed, err := c.Reserve(ctx, "clientA", 1)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if allowed {
		t.Error("a subsequent Reserve after crossing the cap must return false")
	}
}

func TestInProcessTouchCounter_WindowResets(t *testing.T) {
	c := NewInProcessTouchCounter(1, time.Hour)
	fakeNow := time.Now()
	c.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := c.Reserve(ctx, "clientA", 1); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := c.Settle(ctx, "clientA", []string{"r1"}, 1); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if allowed, _ := c.Reserve(ctx, "clientA", 1); allowed {
		t.Fatal("should be denied at the cap within the same window")
	}

	fakeNow = fakeNow.Add(time.Hour + time.Second)
	if allowed, _ := c.Reserve(ctx, "clientA", 1); !allowed {
		t.Error("should be allowed again once the rolling window has passed")
	}
}

func TestInProcessTouchCounter_KeyedPerSub(t *testing.T) {
	c := NewInProcessTouchCounter(1, 24*time.Hour)
	ctx := context.Background()
	if _, err := c.Reserve(ctx, "clientA", 1); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := c.Settle(ctx, "clientA", []string{"r1"}, 1); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	allowed, err := c.Reserve(ctx, "clientB", 1)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !allowed {
		t.Error("a different sub must have its own independent window")
	}
}

// TestInProcessTouchCounter_SweepEvictsStaleWindows covers the unbounded
// map growth Nolan Reyes flagged (PR #26 review): a sub that stops
// calling must eventually be evicted from the map, not held forever.
func TestInProcessTouchCounter_SweepEvictsStaleWindows(t *testing.T) {
	c := NewInProcessTouchCounter(10, time.Hour)
	fakeNow := time.Now()
	c.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := c.Reserve(ctx, "clientA", 1); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := c.Settle(ctx, "clientA", []string{"r1"}, 1); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if _, ok := c.windows["clientA"]; !ok {
		t.Fatal("expected an entry for clientA before any sweep")
	}

	// Advance well past 2x the window and trigger a sweep via any Reserve
	// call (sweepLocked runs opportunistically, gated on lastSweep age).
	fakeNow = fakeNow.Add(3 * time.Hour)
	if _, err := c.Reserve(ctx, "clientB", 1); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if _, ok := c.windows["clientA"]; ok {
		t.Error("a stale window (2x+ the window duration old) should have been swept")
	}
}
