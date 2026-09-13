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

// identityOverflowBaseBackoff/MaxBackoff/AlertThreshold govern the
// SHARED overflow bucket (below) — deliberately more aggressive than
// the per-identity constants above: this bucket protects against every
// identity that couldn't get its own slot at once, so it should trip
// faster and stay tripped longer than any single identity's own bucket
// would (Marcus Ilori's ruling, PR #40: "fail-toward-stricter under
// pressure, not fail-open... a shared, aggressive rate limit instead of
// no limit at all").
const (
	identityOverflowBaseBackoff    = 5 * time.Second
	identityOverflowMaxBackoff     = 10 * time.Minute
	identityOverflowAlertThreshold = 3
)

// identityOverflowKey is the shared bucket's single, fixed key — used
// via the overflow limiter's own clientID/sourceIP slots exactly like
// any per-identity key is, but always this one fixed value, never a
// hash of a real identity.
const identityOverflowKey = "identity-limiter-overflow"

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
//
// Past this cap, a genuinely new identity does NOT bypass identity-level
// limiting entirely — Marcus Ilori's ruling rejected that as
// "degrading to the exact original weakness Tomasz found," since a cap
// an attacker can cheaply fill (no valid vendor credential needed) would
// just delay, not close, the stolen-identities scenario. Instead it
// falls into the shared overflow bucket below: coarser (many identities
// share one bucket once the table is full), but never zero protection.
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

// hashIdentity derives this limiter's map key from callerSub plus one
// or more identity components (a single vendor username for /auth; a
// phone AND name passed as two SEPARATE components for /identity,
// rather than pre-joined with a printable separator) — HMAC-SHA256,
// never the raw attempted identity held as a map key or ever logged
// (Tomasz Wrede's review, PR #40: "a salted/hashed submitted identity,
// never the raw username/phone/whatever's being tried against the
// vendor"). Composite over BOTH callerSub and the identity components —
// per Marcus Ilori's ruling — so two different internal callers
// submitting the same identity never share one bucket, even though this
// project's actual topology has only one internal caller today.
//
// Each component is written with its own null-byte separator rather
// than concatenated by the caller first: passing body.Phone and
// body.Name as two separate arguments here (instead of joining them
// with a printable delimiter like "|" before calling this function)
// avoids a delimiter collision with a printable character that's
// actually plausible in real phone/name input, narrowing (not fully
// eliminating — a null byte could theoretically still appear in an
// input) the same class of concatenation ambiguity a naive join would
// have (Oren Castellan, PR #40 review).
func hashIdentity(callerSub string, identityParts ...string) string {
	mac := hmac.New(sha256.New, identityHMACKey)
	mac.Write([]byte(callerSub))
	for _, part := range identityParts {
		mac.Write([]byte{0})
		mac.Write([]byte(part))
	}
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
// A second, separate GrantLimiter instance backs the shared overflow
// bucket past maxTrackedIdentities — its own key space is exactly one
// fixed value (identityOverflowKey), so it's bounded by construction
// regardless of how many distinct identities overflow into it.
type identityRateLimiter struct {
	underlying *api.InProcessGrantLimiter // per-identity buckets, for tracked keys
	overflow   *api.InProcessGrantLimiter // one shared bucket for everything past the cap

	mu        sync.Mutex
	trackedAt map[string]time.Time
	lastSweep time.Time
}

func NewIdentityRateLimiter() *identityRateLimiter {
	return &identityRateLimiter{
		underlying: api.NewInProcessGrantLimiter(
			identityLimiterBaseBackoff, identityLimiterMaxBackoff, identityLimiterAlertThreshold, nil,
		),
		overflow: api.NewInProcessGrantLimiter(
			identityOverflowBaseBackoff, identityOverflowMaxBackoff, identityOverflowAlertThreshold, nil,
		),
		trackedAt: make(map[string]time.Time),
	}
}

// trackOrCheck is the ONE place that decides whether key is "tracked" —
// called from allow (where a not-yet-tracked key is admitted if there's
// room under maxTrackedIdentities) and from recordFailure/recordSuccess
// (where a not-tracked key is never admitted; record* never adds a new
// entry on its own). This single choke point is what actually enforces
// the memory bound: recordFailure/recordSuccess used to skip this check
// entirely and call straight into the wrapped api.GrantLimiter, so an
// attacker flooding past the cap with fresh failing identities grew that
// limiter's own byClientID/bySourceIP maps without limit even though
// allow() itself correctly degraded open — the cap was checked in the
// one place that doesn't retain state and skipped in the two places
// that do (Nolan Reyes, PR #40 review). Now every one of the three entry
// points agrees on the same tracked/not-tracked answer for a given key
// at a given moment, and only a tracked key ever reaches the wrapped
// limiter at all.
func (l *identityRateLimiter) trackOrCheck(key string, admitIfRoom bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.sweepLocked(now)

	if _, tracked := l.trackedAt[key]; tracked {
		l.trackedAt[key] = now // refresh: this is activity, extend the TTL
		return true
	}
	if !admitIfRoom || len(l.trackedAt) >= maxTrackedIdentities {
		return false
	}
	l.trackedAt[key] = now
	return true
}

// allow reports whether an attempt against attemptedIdentity by
// callerSub may proceed. Once maxTrackedIdentities distinct keys are
// already tracked, a genuinely NEW identity does NOT bypass
// identity-level limiting — it falls into the shared overflow bucket
// instead (Marcus Ilori's ruling: fail toward stricter under pressure,
// not open, matching this project's own pattern elsewhere — JWKS fails
// toward rejection past its staleness bound, the distinct-record
// counter hard-denies rather than merely alerting). Existing tracked
// identities keep their own individual buckets, unaffected.
func (l *identityRateLimiter) allow(ctx context.Context, callerSub string, identityParts ...string) (bool, error) {
	key := hashIdentity(callerSub, identityParts...)
	if l.trackOrCheck(key, true) {
		return l.underlying.Allow(ctx, key, key)
	}
	return l.overflow.Allow(ctx, identityOverflowKey, identityOverflowKey)
}

// recordFailure/recordSuccess never admit a new key into the per-identity
// tracked set on their own — only allow() (called first, on every
// request, per handlers.go's wiring) decides whether a key gets its own
// slot. A key that allow() routed to the overflow bucket is recorded
// there too, keeping the per-identity limiter's own maps bounded by
// maxTrackedIdentities regardless of how many distinct identities an
// attacker generates, while the shared overflow bucket still
// accumulates failures across all of them (bounded by construction: its
// key space is exactly one fixed value).
func (l *identityRateLimiter) recordFailure(ctx context.Context, callerSub string, identityParts ...string) {
	key := hashIdentity(callerSub, identityParts...)
	if l.trackOrCheck(key, false) {
		l.underlying.RecordFailure(ctx, key, key)
		return
	}
	l.overflow.RecordFailure(ctx, identityOverflowKey, identityOverflowKey)
}

func (l *identityRateLimiter) recordSuccess(ctx context.Context, callerSub string, identityParts ...string) {
	key := hashIdentity(callerSub, identityParts...)
	if l.trackOrCheck(key, false) {
		l.underlying.RecordSuccess(ctx, key, key)
		return
	}
	l.overflow.RecordSuccess(ctx, identityOverflowKey, identityOverflowKey)
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
