package connector

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"loginid-takehome/internal/api"
)

// identityLimiterBaseBackoff/MaxBackoff/AlertThreshold are this
// component's own engineering defaults, same reasoning as
// connectorVendorLimiterBaseBackoff/etc. in internal/app/connector_router.go
// — flagged for 02's review rather than treated as ruled numbers.
const (
	identityLimiterBaseBackoff    = 1 * time.Second
	identityLimiterMaxBackoff     = 5 * time.Minute
	identityLimiterAlertThreshold = 10
)

// maxTrackedIdentities bounds this limiter's own tracked-key set
// independently of api.GrantLimiter's time-based sweep — the same
// "bounded distinct-key cardinality" pattern LT-40's JWKSCache already
// uses for unknown kids (jwksMaxTrackedUnknownKids), extended here to a
// much larger value: unlike a kid (a small, bounded set of currently
// valid signing keys), a legitimate identity space can genuinely be
// large (many real vendor end-users), so capping at JWKS's own
// small number would fail open on ordinary traffic almost immediately.
// This is this component's own engineering default (flagged for 02's
// review), not a ruled number.
const maxTrackedIdentities = 10000

// identityTrackedKeyTTL/identitySweepInterval bound how long a tracked
// identity key survives with no further activity — same sweep-based
// eviction shape as InProcessGrantLimiter/InProcessTouchCounter/
// JWKSCache's own unknown-kid map, so a burst of one-off garbage
// identities doesn't permanently consume tracked-key slots.
const (
	identityTrackedKeyTTL = time.Hour
	identitySweepInterval = time.Minute
)

// identityHMACKey is generated once per process — this limiter's keys
// never need to survive a restart (the state itself is in-process and
// ephemeral), so a random key generated at process start is sufficient
// and avoids embedding any fixed secret in source.
var identityHMACKey = mustRandomBytes(32)

func mustRandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("connector: generating identity-limiter HMAC key: " + err.Error())
	}
	return b
}

// hashIdentity derives this limiter's map key from (callerSub,
// attemptedIdentity) — HMAC-SHA256, never the raw attempted identity
// (a vendor username, or a phone/name pair) held as a map key or ever
// logged (Tomasz Wrede's review, PR #40: "a salted/hashed submitted
// identity, never the raw username/phone/whatever's being tried against
// the vendor"). Composite over BOTH callerSub and attemptedIdentity —
// per Marcus Ilori's ruling — so two different internal callers
// submitting the same identity string never share one bucket, even
// though this project's actual topology has only one internal caller
// today.
func hashIdentity(callerSub, attemptedIdentity string) string {
	mac := hmac.New(sha256.New, identityHMACKey)
	mac.Write([]byte(callerSub))
	mac.Write([]byte{0}) // separator: prevents ("ab","c") colliding with ("a","bc")
	mac.Write([]byte(attemptedIdentity))
	return hex.EncodeToString(mac.Sum(nil))
}

// identityRateLimiter is attack-tree leaf 5's missing second dimension
// (Tomasz Wrede's cold review of PR #40): connector-security.md's own
// ruled control specifies backoff "per identity attempted, not just per
// client" — the caller-level GrantLimiter instance
// (internal/app/connector_router.go) alone means a single success
// anywhere resets that caller's ENTIRE consecutive-failure count,
// letting a compromised caller cycle through many stolen identities
// indefinitely as long as it occasionally succeeds against one of them.
// This type tracks backoff per (caller, attempted identity) instead, so
// retrying the SAME identity trips independently of whether some OTHER
// identity under the same caller just succeeded — and, symmetrically,
// one legitimate user's typo never affects a different legitimate
// user's own attempt under the same caller (both are named,
// enforcement-tested properties, not just a design intent).
//
// Wraps api.GrantLimiter (not a new backoff mechanism — the hashed
// identity is used as BOTH of GrantLimiter's own two key slots, since
// neither "client_id" nor "source IP" is the right name for what this
// tracks; using the same value for both is harmless, just double
// bookkeeping for one entity) with an explicit, bounded tracked-key set
// so an attacker submitting unbounded distinct garbage identities can't
// grow memory without limit between GrantLimiter's own periodic sweeps.
type identityRateLimiter struct {
	underlying *api.InProcessGrantLimiter

	mu        sync.Mutex
	trackedAt map[string]time.Time
	lastSweep time.Time
}

func NewIdentityRateLimiter() *identityRateLimiter {
	return &identityRateLimiter{
		underlying: api.NewInProcessGrantLimiter(
			identityLimiterBaseBackoff, identityLimiterMaxBackoff, identityLimiterAlertThreshold, nil,
		),
		trackedAt: make(map[string]time.Time),
	}
}

// allow reports whether an attempt against attemptedIdentity by
// callerSub may proceed. Once maxTrackedIdentities distinct keys are
// already tracked, a genuinely NEW identity is allowed through this
// limiter (fails open on THIS dimension only) rather than blocking
// every new identity outright — the caller-level GrantLimiter still
// provides a base rate limit regardless, so this degrades to
// caller-level-only protection under a tracked-key flood rather than
// denying service to legitimate new identities (same reasoning as
// JWKSCache's own bounded unknown-kid cap).
func (l *identityRateLimiter) allow(ctx context.Context, callerSub, attemptedIdentity string) (bool, error) {
	key := hashIdentity(callerSub, attemptedIdentity)

	l.mu.Lock()
	now := time.Now()
	l.sweepLocked(now)
	if _, tracked := l.trackedAt[key]; !tracked {
		if len(l.trackedAt) >= maxTrackedIdentities {
			l.mu.Unlock()
			return true, nil
		}
		l.trackedAt[key] = now
	} else {
		l.trackedAt[key] = now
	}
	l.mu.Unlock()

	return l.underlying.Allow(ctx, key, key)
}

func (l *identityRateLimiter) recordFailure(ctx context.Context, callerSub, attemptedIdentity string) {
	key := hashIdentity(callerSub, attemptedIdentity)
	l.underlying.RecordFailure(ctx, key, key)
}

func (l *identityRateLimiter) recordSuccess(ctx context.Context, callerSub, attemptedIdentity string) {
	key := hashIdentity(callerSub, attemptedIdentity)
	l.underlying.RecordSuccess(ctx, key, key)
}

func (l *identityRateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < identitySweepInterval {
		return
	}
	l.lastSweep = now
	for k, seenAt := range l.trackedAt {
		if now.Sub(seenAt) >= identityTrackedKeyTTL {
			delete(l.trackedAt, k)
		}
	}
}
