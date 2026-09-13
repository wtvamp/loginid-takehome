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

// TestIdentityRateLimiter_BoundedCardinality_OverflowsToSharedBucket is
// Marcus Ilori's ruling (PR #40): past maxTrackedIdentities, a new
// identity does NOT bypass identity-level limiting — it falls into a
// shared overflow bucket instead. First proves a brand-new identity past
// the cap is still allowed on its FIRST attempt (the overflow bucket
// starts fresh, same as any bucket would); then proves the bucket is
// genuinely SHARED — failures recorded against one overflowing identity
// eventually block a DIFFERENT overflowing identity too, which is
// exactly the "never zero protection" property a plain fail-open design
// would not have.
func TestIdentityRateLimiter_BoundedCardinality_OverflowsToSharedBucket(t *testing.T) {
	l := NewIdentityRateLimiter()
	ctx := context.Background()

	// Flood past maxTrackedIdentities with distinct identities, each
	// only ever calling allow() (no failures recorded) — fills the
	// per-identity tracked set without touching the overflow bucket.
	for i := 0; i < maxTrackedIdentities+10; i++ {
		if _, err := l.allow(ctx, "flooding-caller", string(rune(i))+"-flood"); err != nil {
			t.Fatalf("allow: %v", err)
		}
	}

	// A brand-new identity past the cap is allowed on its first attempt
	// — the shared overflow bucket itself hasn't been tripped yet.
	allowed, err := l.allow(ctx, "flooding-caller", "overflow-identity-1")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !allowed {
		t.Fatalf("a new identity past maxTrackedIdentities was denied on its FIRST attempt — the shared overflow bucket should start unblocked")
	}
	l.recordFailure(ctx, "flooding-caller", "overflow-identity-1")

	// A DIFFERENT identity, also past the cap, must eventually be
	// blocked by the SAME shared bucket — proving this isn't a
	// per-overflow-identity allowance in disguise, and that the "never
	// zero protection" property actually holds: enough overflow traffic
	// (from any mix of identities) still trips real limiting.
	blockedAfter := -1
	for i := 0; i < 10; i++ {
		ok, err := l.allow(ctx, "flooding-caller", "overflow-identity-2")
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if !ok {
			blockedAfter = i
			break
		}
		l.recordFailure(ctx, "flooding-caller", "overflow-identity-2")
	}
	if blockedAfter == -1 {
		t.Fatalf("the shared overflow bucket never blocked anything across 10 consecutive overflow failures — overflow traffic is getting no protection at all")
	}
}

// TestIdentityRateLimiter_TrackedIdentityUnaffectedByOverflowPressure
// confirms an identity that DID get its own tracked slot (within the
// cap) is completely unaffected by however much unrelated traffic is
// hammering the shared overflow bucket — the two are genuinely
// independent limiters, not one shared fate.
func TestIdentityRateLimiter_TrackedIdentityUnaffectedByOverflowPressure(t *testing.T) {
	l := NewIdentityRateLimiter()
	ctx := context.Background()

	// A legitimately-tracked identity, within the cap.
	if allowed, err := l.allow(ctx, "caller", "legit-user"); err != nil || !allowed {
		t.Fatalf("legit-user first attempt: allowed=%v err=%v", allowed, err)
	}

	// Drive the shared overflow bucket into its blocked state with a
	// flood of DIFFERENT, never-tracked identities under the same cap
	// pressure (reuse the same flood as the sibling test, cheaply, by
	// filling the tracked set first).
	for i := 0; i < maxTrackedIdentities; i++ {
		if _, err := l.allow(ctx, "caller", string(rune(i))+"-fill"); err != nil {
			t.Fatalf("allow: %v", err)
		}
	}
	for i := 0; i < 10; i++ {
		ok, err := l.allow(ctx, "caller", "overflow-attacker")
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
		if !ok {
			break
		}
		l.recordFailure(ctx, "caller", "overflow-attacker")
	}

	// legit-user's OWN tracked bucket must be untouched by any of that.
	allowed, err := l.allow(ctx, "caller", "legit-user")
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !allowed {
		t.Errorf("legit-user (a tracked, within-cap identity) was blocked by unrelated overflow-bucket pressure — the two must be independent")
	}
}
