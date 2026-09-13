package connector

import (
	"context"
	"testing"
)

// TestIdentityRateLimiter_StolenIdentitiesSurviveAnUnrelatedSuccess is
// Tomasz Wrede's named attack scenario (cold review of PR #40): the
// caller-level GrantLimiter alone resets its ENTIRE consecutive-failure
// count on any success, so a compromised caller cycling through many
// stolen identities can keep a fresh allowance indefinitely as long as
// it occasionally succeeds against ONE of them (e.g. one seed valid
// account). Per-(caller, identity) keying must not have that property:
// a specific identity's own backoff must trip on its own repeated
// failures and must NOT be reset by a completely different identity
// succeeding under the same caller.
func TestIdentityRateLimiter_StolenIdentitiesSurviveAnUnrelatedSuccess(t *testing.T) {
	l := NewIdentityRateLimiter()
	ctx := context.Background()
	const caller = "api-service"

	// Fail against "victim-1" enough times to trip its own backoff.
	blockedAfter := -1
	for i := 0; i < 10; i++ {
		allowed, err := l.allow(ctx, caller, "victim-1")
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if !allowed {
			blockedAfter = i
			break
		}
		l.recordFailure(ctx, caller, "victim-1")
	}
	if blockedAfter == -1 {
		t.Fatalf("victim-1 was never blocked across 10 consecutive failures")
	}

	// The attacker now succeeds against a COMPLETELY DIFFERENT identity
	// under the same caller (the "one valid seed account" case).
	allowed, err := l.allow(ctx, caller, "victim-2")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !allowed {
		t.Fatalf("victim-2 (never attempted before) was blocked — test setup bug")
	}
	l.recordSuccess(ctx, caller, "victim-2")

	// victim-1 must STILL be blocked — victim-2's success must not have
	// reset it. This is exactly the property a single shared per-caller
	// bucket does NOT have (a success there resets everything).
	allowed, err = l.allow(ctx, caller, "victim-1")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if allowed {
		t.Errorf("victim-1 is allowed again after an unrelated identity (victim-2) succeeded — a per-identity bucket must not be resettable by a different identity's success")
	}
}

// TestIdentityRateLimiter_TypoIsolation is Tomasz Wrede's other named
// scenario: two different LEGITIMATE users behind the same caller, one
// of whom mistypes their credential once, must not affect the other —
// "strangers' typos" must not over-limit an unrelated identity.
func TestIdentityRateLimiter_TypoIsolation(t *testing.T) {
	l := NewIdentityRateLimiter()
	ctx := context.Background()
	const caller = "api-service"

	// user-A mistypes once — this correctly starts user-A's OWN backoff
	// (same progressive-backoff-from-the-first-failure design as
	// api.GrantLimiter elsewhere in this codebase; not itself the
	// property under test here) — then eventually succeeds.
	if allowed, err := l.allow(ctx, caller, "user-A"); err != nil || !allowed {
		t.Fatalf("user-A first attempt: allowed=%v err=%v", allowed, err)
	}
	l.recordFailure(ctx, caller, "user-A")
	l.recordSuccess(ctx, caller, "user-A")

	// user-B, a completely different identity under the same caller,
	// must never have been affected by user-A's typo — first attempt
	// must be allowed.
	allowed, err := l.allow(ctx, caller, "user-B")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !allowed {
		t.Errorf("user-B was blocked by user-A's unrelated typo under the same caller — per-identity isolation must prevent this")
	}
}

func TestIdentityRateLimiter_DifferentCallersSameIdentityDoNotShareState(t *testing.T) {
	l := NewIdentityRateLimiter()
	ctx := context.Background()

	// caller-1 fails repeatedly against "shared-name" (both callers
	// happen to submit the same attempted-identity string).
	for i := 0; i < 10; i++ {
		allowed, err := l.allow(ctx, "caller-1", "shared-name")
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if !allowed {
			break
		}
		l.recordFailure(ctx, "caller-1", "shared-name")
	}

	// caller-2 attempting the SAME identity string must not be affected
	// — the key is composite over (caller, identity), per Marcus Ilori's
	// ruling, not identity alone.
	allowed, err := l.allow(ctx, "caller-2", "shared-name")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !allowed {
		t.Errorf("caller-2 was blocked by caller-1's failures against the same identity string — the key must be composite over (caller, identity)")
	}
}

func TestIdentityRateLimiter_BoundedCardinality_DegradesOpenNotClosed(t *testing.T) {
	l := NewIdentityRateLimiter()
	ctx := context.Background()

	// Flood past maxTrackedIdentities with distinct identities.
	for i := 0; i < maxTrackedIdentities+10; i++ {
		if _, err := l.allow(ctx, "flooding-caller", string(rune(i))+"-flood"); err != nil {
			t.Fatalf("allow: %v", err)
		}
	}

	// A brand-new identity past the cap must still be ALLOWED (fails
	// open on this dimension only, per this type's own doc comment) —
	// never silently denying service to a legitimate new identity just
	// because the tracked-key set is full.
	allowed, err := l.allow(ctx, "flooding-caller", "one-more-new-identity")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !allowed {
		t.Errorf("a new identity past maxTrackedIdentities was denied — this dimension must fail OPEN (allow), not closed, once the tracked-key cap is reached")
	}
}
