package api

import (
	"context"
	"testing"
	"time"
)

func TestInProcessTouchCounter_CountsDistinctRecordsOnly(t *testing.T) {
	c := NewInProcessTouchCounter(3, 24*time.Hour)
	ctx := context.Background()

	n, err := c.RecordTouches(ctx, "clientA", []string{"r1", "r2", "r1"})
	if err != nil {
		t.Fatalf("RecordTouches: %v", err)
	}
	if n != 2 {
		t.Errorf("count after recording [r1,r2,r1] = %d, want 2 (distinct)", n)
	}
}

// TestInProcessTouchCounter_HardDeniesAfterCrossingCap is
// handoff-03-auth.md v3's ruling: crossing the cap hard-denies further
// requests for the remainder of the window — the request that crosses
// the cap itself still completes, but the next one is denied.
func TestInProcessTouchCounter_HardDeniesAfterCrossingCap(t *testing.T) {
	c := NewInProcessTouchCounter(2, 24*time.Hour)
	ctx := context.Background()

	allowed, err := c.Allowed(ctx, "clientA")
	if err != nil || !allowed {
		t.Fatalf("first check: allowed=%v err=%v, want true/nil", allowed, err)
	}
	if _, err := c.RecordTouches(ctx, "clientA", []string{"r1", "r2", "r3"}); err != nil {
		t.Fatalf("RecordTouches: %v", err)
	}
	// This request's own 3 touches pushed the count to 3, over the cap
	// of 2 — but this request had already been allowed to proceed before
	// RecordTouches ran, per the "further requests" wording.
	allowed, err = c.Allowed(ctx, "clientA")
	if err != nil {
		t.Fatalf("Allowed: %v", err)
	}
	if allowed {
		t.Error("a subsequent check after crossing the cap must return false")
	}
}

func TestInProcessTouchCounter_WindowResets(t *testing.T) {
	c := NewInProcessTouchCounter(1, time.Hour)
	fakeNow := time.Now()
	c.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := c.RecordTouches(ctx, "clientA", []string{"r1"}); err != nil {
		t.Fatalf("RecordTouches: %v", err)
	}
	if allowed, _ := c.Allowed(ctx, "clientA"); allowed {
		t.Fatal("should be denied at the cap within the same window")
	}

	fakeNow = fakeNow.Add(time.Hour + time.Second)
	if allowed, _ := c.Allowed(ctx, "clientA"); !allowed {
		t.Error("should be allowed again once the rolling window has passed")
	}
}

func TestInProcessTouchCounter_KeyedPerSub(t *testing.T) {
	c := NewInProcessTouchCounter(1, 24*time.Hour)
	ctx := context.Background()
	if _, err := c.RecordTouches(ctx, "clientA", []string{"r1"}); err != nil {
		t.Fatalf("RecordTouches: %v", err)
	}
	allowed, err := c.Allowed(ctx, "clientB")
	if err != nil {
		t.Fatalf("Allowed: %v", err)
	}
	if !allowed {
		t.Error("a different sub must have its own independent window")
	}
}
