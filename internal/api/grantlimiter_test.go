package api

import (
	"context"
	"testing"
	"time"
)

func TestGrantLimiter_AllowsFirstAttempt(t *testing.T) {
	l := NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)
	allow, err := l.Allow(context.Background(), "client-a", "1.2.3.4")
	if err != nil || !allow {
		t.Fatalf("Allow = %v, %v; want true, nil", allow, err)
	}
}

func TestGrantLimiter_BlocksAfterFailure(t *testing.T) {
	l := NewInProcessGrantLimiter(time.Minute, time.Hour, 5, nil)
	ctx := context.Background()
	l.RecordFailure(ctx, "client-a", "1.2.3.4")

	allow, err := l.Allow(ctx, "client-a", "1.2.3.4")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allow {
		t.Error("should be blocked immediately after a failure, within the backoff window")
	}
}

func TestGrantLimiter_BackoffIsExponential(t *testing.T) {
	l := NewInProcessGrantLimiter(time.Second, time.Hour, 100, nil)
	fakeNow := time.Now()
	l.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	l.RecordFailure(ctx, "client-a", "1.2.3.4")
	// First failure: 1s backoff. Advance 1.5s — should be allowed again.
	fakeNow = fakeNow.Add(1500 * time.Millisecond)
	if allow, _ := l.Allow(ctx, "client-a", "1.2.3.4"); !allow {
		t.Fatal("should be allowed again after the first (1s) backoff elapses")
	}

	l.RecordFailure(ctx, "client-a", "1.2.3.4")
	// Second consecutive failure: 2s backoff. Advance only 1.5s again —
	// should still be blocked, proving the window grew.
	fakeNow = fakeNow.Add(1500 * time.Millisecond)
	if allow, _ := l.Allow(ctx, "client-a", "1.2.3.4"); allow {
		t.Error("should still be blocked — the second failure's backoff (2s) hasn't elapsed yet")
	}
}

func TestGrantLimiter_BackoffCappedAtMax(t *testing.T) {
	l := NewInProcessGrantLimiter(time.Second, 5*time.Second, 100, nil)
	fakeNow := time.Now()
	l.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		l.RecordFailure(ctx, "client-a", "1.2.3.4")
	}
	// Even after many failures, the backoff must not exceed maxBackoff.
	fakeNow = fakeNow.Add(5*time.Second + time.Millisecond)
	if allow, _ := l.Allow(ctx, "client-a", "1.2.3.4"); !allow {
		t.Error("backoff should be capped at maxBackoff (5s), not grow unbounded across 10 failures")
	}
}

func TestGrantLimiter_KeyedIndependentlyByClientIDAndSourceIP(t *testing.T) {
	l := NewInProcessGrantLimiter(time.Hour, time.Hour, 100, nil)
	ctx := context.Background()

	// A distributed brute force: many source IPs against one client_id.
	// The client_id key alone must block a request even from a fresh IP.
	l.RecordFailure(ctx, "client-a", "1.1.1.1")
	if allow, _ := l.Allow(ctx, "client-a", "9.9.9.9"); allow {
		t.Error("a blocked client_id must block attempts from a different source IP too")
	}

	// A single-source credential-stuffing run: many client_ids from one
	// IP. The source-IP key alone must block a fresh client_id from that
	// same IP.
	if allow, _ := l.Allow(ctx, "client-b", "1.1.1.1"); allow {
		t.Error("a blocked source IP must block attempts for a different client_id too")
	}

	// A genuinely unrelated (client_id, IP) pair must not be affected.
	if allow, _ := l.Allow(ctx, "client-c", "2.2.2.2"); !allow {
		t.Error("an unrelated client_id/source IP pair must not be blocked")
	}
}

func TestGrantLimiter_SuccessResetsBackoff(t *testing.T) {
	l := NewInProcessGrantLimiter(time.Hour, time.Hour, 100, nil)
	ctx := context.Background()

	l.RecordFailure(ctx, "client-a", "1.2.3.4")
	if allow, _ := l.Allow(ctx, "client-a", "1.2.3.4"); allow {
		t.Fatal("should be blocked after a failure")
	}

	l.RecordSuccess(ctx, "client-a", "1.2.3.4")
	if allow, _ := l.Allow(ctx, "client-a", "1.2.3.4"); !allow {
		t.Error("a successful grant should reset backoff state entirely")
	}
}

// TestGrantLimiter_AlertsOnceOnSustainedFailureRun is
// handoff-03-auth.md v3's "alerting on a sustained run of failed grants
// against one client_id" — fired exactly once per threshold crossing,
// not once per failure past it (which would just be alert-fatigue noise
// indistinguishable from the failures themselves).
func TestGrantLimiter_AlertsOnceOnSustainedFailureRun(t *testing.T) {
	var alerts []int
	l := NewInProcessGrantLimiter(time.Nanosecond, time.Nanosecond, 3, func(clientID string, consecutiveFailures int) {
		alerts = append(alerts, consecutiveFailures)
	})
	fakeNow := time.Now()
	l.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	for i := 0; i < 6; i++ {
		l.RecordFailure(ctx, "client-a", "1.2.3.4")
		fakeNow = fakeNow.Add(time.Millisecond) // clear the (nanosecond) backoff each time
	}

	if len(alerts) != 1 {
		t.Fatalf("alerts fired = %v, want exactly one (at the threshold crossing)", alerts)
	}
	if alerts[0] != 3 {
		t.Errorf("alert fired at %d consecutive failures, want at the threshold (3)", alerts[0])
	}
}

func TestGrantLimiter_AlertOnlyNamesClientID_NotSourceIP(t *testing.T) {
	// The requirement names client_id specifically for alerting — a
	// source IP hitting the threshold across many different client_ids
	// (each individually under threshold) is a different signal
	// (distributed credential stuffing) this limiter still blocks via
	// the source-IP key, but doesn't claim to alert on by itself.
	var alerted bool
	l := NewInProcessGrantLimiter(time.Nanosecond, time.Nanosecond, 3, func(clientID string, n int) {
		alerted = true
	})
	fakeNow := time.Now()
	l.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	clientIDs := []string{"client-a", "client-b", "client-c"}
	for _, id := range clientIDs {
		l.RecordFailure(ctx, id, "1.2.3.4")
		fakeNow = fakeNow.Add(time.Millisecond)
	}
	if alerted {
		t.Error("alertFn should be keyed on a single client_id's consecutive failures, not fired for varying client_ids sharing a source IP")
	}
}
