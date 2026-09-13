package api

import (
	"context"
	"sync"
	"time"
)

// GrantLimiter enforces the token-issuing endpoint's own failed-grant
// rate limiting and progressive backoff — handoff-03-auth.md v3:
// "Token-endpoint brute-force protection, separate from the resource-API
// rate limits ... failed-grant rate limiting and progressive backoff per
// client_id and per source IP on the token-issuing endpoint itself, plus
// alerting on a sustained run of failed grants against one client_id."
//
// This is genuinely separate from RateLimiter (ratelimit.go): that type
// gates the resource API by an already-authenticated sub, at a fixed
// per-minute rate; this one gates the endpoint that issues the
// credential in the first place, keyed independently by client_id AND
// by source IP — a distributed brute force (many IPs, one client_id)
// and a single-source credential-stuffing run (one IP, many client_ids)
// are both attack shapes this is meant to slow down, and either key
// exceeding its own backoff blocks the attempt.
//
// Not wired to a live endpoint by this story: /auth/token's real grant
// logic is LT-51's scope (this story's non-goals). This is the
// ready-to-use component LT-51 constructs and calls from the real
// handler — building it now, tested in isolation, means LT-51 doesn't
// also have to design a backoff/alerting scheme under its own deadline.
type GrantLimiter interface {
	// Allow reports whether a grant attempt for clientID from sourceIP
	// may proceed right now — false while either key is in backoff.
	Allow(ctx context.Context, clientID, sourceIP string) (bool, error)
	// RecordFailure records a failed grant attempt for both clientID and
	// sourceIP, advancing each key's backoff state independently.
	// Crossing the alert threshold for a given client_id (not source IP
	// — the requirement names client_id specifically) fires alert
	// exactly once per crossing, not once per subsequent failure.
	RecordFailure(ctx context.Context, clientID, sourceIP string)
	// RecordSuccess clears backoff state for both clientID and
	// sourceIP — a successful grant resets the count, standard
	// exponential-backoff practice: this deters sustained brute force,
	// it does not permanently degrade a legitimate client that mistyped
	// a secret once.
	RecordSuccess(ctx context.Context, clientID, sourceIP string)
}

// grantBackoffState is one key's (a client_id's, or a source IP's)
// consecutive-failure bookkeeping.
type grantBackoffState struct {
	consecutiveFailures int
	blockedUntil        time.Time
	alerted             bool
}

// InProcessGrantLimiter is an in-process implementation. As with
// RateLimiter/TouchCounter's in-process implementations, a shared store
// is the multi-instance replacement behind the same interface — the
// mechanism is this track's call, per handoff-03-auth.md v3.
//
// Both maps are swept periodically (see sweepLocked) rather than left to
// grow forever: this component's own doc comment names "a distributed
// brute force (many IPs, one client_id)" as a defended attack shape, and
// that is exactly the traffic pattern that would otherwise grow
// bySourceIP without bound — every distinct attacking IP earns a
// permanent entry that only a matching RecordSuccess would remove, which
// an attacking IP by definition never triggers (Nolan Reyes, PR #30
// review — the same unbounded-growth shape an earlier PR's TouchCounter
// review already found and fixed once).
type InProcessGrantLimiter struct {
	mu             sync.Mutex
	byClientID     map[string]*grantBackoffState
	bySourceIP     map[string]*grantBackoffState
	baseBackoff    time.Duration
	maxBackoff     time.Duration
	alertThreshold int
	alertFn        func(clientID string, consecutiveFailures int)
	now            func() time.Time
	lastSweep      time.Time
}

// grantSweepInterval and grantEntryTTL bound how long a key's state
// survives with no further failures against it. A blocked key is never
// swept before its own backoff has elapsed (sweeping mid-block would
// silently un-block it early); grantEntryTTL is generous past any
// realistic maxBackoff so a legitimately still-escalating attacker isn't
// reset early either.
const (
	grantSweepInterval = time.Minute
	grantEntryTTL      = time.Hour
)

// NewInProcessGrantLimiter constructs a limiter with exponential backoff
// starting at baseBackoff (doubling per consecutive failure, capped at
// maxBackoff) and alertFn called once per client_id crossing
// alertThreshold consecutive failures. alertFn may be nil (no alerting —
// tests use this; production wiring passes a real sink).
func NewInProcessGrantLimiter(baseBackoff, maxBackoff time.Duration, alertThreshold int, alertFn func(clientID string, consecutiveFailures int)) *InProcessGrantLimiter {
	return &InProcessGrantLimiter{
		byClientID:     make(map[string]*grantBackoffState),
		bySourceIP:     make(map[string]*grantBackoffState),
		baseBackoff:    baseBackoff,
		maxBackoff:     maxBackoff,
		alertThreshold: alertThreshold,
		alertFn:        alertFn,
		now:            time.Now,
	}
}

func (l *InProcessGrantLimiter) Allow(_ context.Context, clientID, sourceIP string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweepLocked(now)
	return !isBlocked(l.byClientID[clientID], now) && !isBlocked(l.bySourceIP[sourceIP], now), nil
}

// sweepLocked drops any entry whose blockedUntil is at least
// grantEntryTTL in the past — called with l.mu already held, at most
// once per grantSweepInterval, so it adds no per-call cost beyond a
// single time comparison on every other call.
func (l *InProcessGrantLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < grantSweepInterval {
		return
	}
	l.lastSweep = now
	sweepMap(l.byClientID, now)
	sweepMap(l.bySourceIP, now)
}

func sweepMap(m map[string]*grantBackoffState, now time.Time) {
	for key, s := range m {
		if now.Sub(s.blockedUntil) >= grantEntryTTL {
			delete(m, key)
		}
	}
}

func isBlocked(s *grantBackoffState, now time.Time) bool {
	return s != nil && now.Before(s.blockedUntil)
}

func (l *InProcessGrantLimiter) RecordFailure(_ context.Context, clientID, sourceIP string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()

	cs := l.advance(l.byClientID, clientID, now)
	l.advance(l.bySourceIP, sourceIP, now)

	if cs.consecutiveFailures >= l.alertThreshold && !cs.alerted {
		cs.alerted = true
		if l.alertFn != nil {
			l.alertFn(clientID, cs.consecutiveFailures)
		}
	}
}

// advance increments key's consecutive-failure count and sets its
// exponential backoff window, creating the state entry if this is its
// first failure.
func (l *InProcessGrantLimiter) advance(m map[string]*grantBackoffState, key string, now time.Time) *grantBackoffState {
	s, ok := m[key]
	if !ok {
		s = &grantBackoffState{}
		m[key] = s
	}
	s.consecutiveFailures++
	backoff := l.baseBackoff << uint(s.consecutiveFailures-1) // 2^(n-1) * base
	if backoff > l.maxBackoff || backoff <= 0 {               // overflow guard: a left-shifted duration can wrap negative
		backoff = l.maxBackoff
	}
	s.blockedUntil = now.Add(backoff)
	return s
}

func (l *InProcessGrantLimiter) RecordSuccess(_ context.Context, clientID, sourceIP string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byClientID, clientID)
	delete(l.bySourceIP, sourceIP)
}
