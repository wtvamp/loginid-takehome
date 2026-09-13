// jwks.go implements a minimal RFC 7517 JSON Web Key Set client: fetch,
// parse RSA public keys, cache with a short TTL. handoff-03-auth.md v4:
// "Public-key distribution for the verifier ... JWKS served by the issuer
// Deployment, reachable only in-cluster via its own ClusterIP Service ...
// Verifier caches the fetched key set in memory with a short TTL (matching
// token TTL is reasonable) to avoid a JWKS fetch on every single
// verification."
package api

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwk is one entry in a JWKS response — only the fields this verifier
// needs to reconstruct an RSA public key (RFC 7517 §4, RFC 7518 §6.3.1).
// Fields this verifier doesn't use (x5c, x5t, ...) are simply not
// unmarshaled; unknown fields in the source JSON are not an error.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"` // base64url, unpadded — the RSA modulus
	E   string `json:"e"` // base64url, unpadded — the RSA public exponent
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

// parseJWKS decodes a JWKS JSON document into a map of kid -> public key.
// Any key with kty != "RSA" is skipped (not an error) — a future non-RSA
// key in the set (e.g. during an algorithm migration) shouldn't break
// verification of tokens signed with the RSA keys that ARE present.
func parseJWKS(data []byte) (map[string]*rsa.PublicKey, error) {
	var resp jwksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("api: parsing JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(resp.Keys))
	for _, k := range resp.Keys {
		if k.Kty != "RSA" {
			continue
		}
		if k.Kid == "" {
			return nil, fmt.Errorf("api: JWKS RSA key missing kid")
		}
		pub, err := rsaPublicKeyFromJWK(k)
		if err != nil {
			return nil, fmt.Errorf("api: JWKS key %q: %w", k.Kid, err)
		}
		keys[k.Kid] = pub
	}
	return keys, nil
}

func rsaPublicKeyFromJWK(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decoding n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decoding e: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, fmt.Errorf("exponent out of range")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// KeySource resolves a kid to the RSA public key that should verify a
// token carrying it. JWKSCache is the real implementation; tests use a
// map-backed fake. ctx carries the calling request's own deadline —
// KeyForKid's real implementation must bound its network I/O by it, not
// by some separate, undocumented timeout (Nolan Reyes, PR #30 review: a
// hung fetch that ignores the caller's context defeats the deadline
// middleware's whole guarantee).
type KeySource interface {
	KeyForKid(ctx context.Context, kid string) (*rsa.PublicKey, error)
}

// ErrJWKSStale is returned by JWKSCache.KeyForKid when the cached key set
// has exceeded its max-staleness bound and could not be refreshed — the
// verifier must fail closed rather than trust keys this old (Tomasz
// Wrede, 02 cold review; PM ruling). Exported so NewJWTMiddleware can log
// a distinct audit event (AuditJWKSUnavailable) for this specific
// failure mode, rather than folding it into an ordinary auth failure.
var ErrJWKSStale = errors.New("api: JWKS cache exceeded max staleness and could not be refreshed")

// jwksMaxStalenessMultiple and jwksUnknownKidRefetchInterval are the
// concrete numbers the PM ruled on for Tomasz Wrede's two findings:
//   - a signing key can be revoked BECAUSE it's compromised; if the JWKS
//     endpoint is also unreachable during that same incident, a
//     stale-cache-forever fallback would keep verifying tokens signed by
//     the revoked key for as long as fetches keep failing. Bounding
//     staleness at a multiple of the normal refresh interval (ttl) turns
//     "fail open forever" into "fail open for a bounded window, then
//     fail closed" — resilient to a brief blip, not to an extended one.
//   - an unknown kid could be a key the issuer rotated in since the last
//     TTL-driven refresh; without an out-of-band refetch, a freshly
//     rotated key would be rejected for up to a full TTL. Rate-limited
//     so an unknown-kid flood can't be used to hammer the issuer's JWKS
//     endpoint.
const (
	jwksMaxStalenessMultiple      = 3
	jwksUnknownKidRefetchInterval = 30 * time.Second

	// jwksMinRefreshRetryInterval floors how often a TTL-expired cache
	// with a just-failed refresh will attempt another real network
	// fetch. Without this, every single request during a sustained
	// issuer outage triggers its own fetch attempt for as long as the
	// outage lasts (up to maxStaleness) — the same "don't hammer a
	// struggling dependency" reasoning already applied to the
	// unknown-kid path, missing from the ordinary refresh path (Oren
	// Castellan, PR #30 re-review). Short relative to ttl/maxStaleness:
	// this only throttles retries during an active failure, it must not
	// delay picking up a healthy issuer again once it recovers.
	jwksMinRefreshRetryInterval = 5 * time.Second

	// jwksMaxTrackedUnknownKids bounds how many distinct kids can hold an
	// outstanding unknown-kid retry reservation at once. Without this,
	// keying the reservation per-kid (see reserveUnknownKidRetry) reopens
	// its own version of the same map-growth concern a flood of DISTINCT
	// garbage kids would create: capped here, so an attacker sending many
	// different fabricated kids can force at most this many extra
	// fetches per jwksUnknownKidRefetchInterval, not one per distinct
	// value they bother to send.
	jwksMaxTrackedUnknownKids = 64
)

// JWKSCache fetches a JWKS document from url and caches the parsed key
// set for ttl, refetching only after it expires — "to avoid a JWKS fetch
// on every single verification" (handoff-03-auth.md v4). Supports at
// least two concurrently valid kids during a rotation overlap window by
// construction: it caches the WHOLE key set the issuer publishes, not
// just the most recent key, so an old and a new key both verify
// successfully for as long as the issuer's own JWKS response lists both.
type JWKSCache struct {
	url          string
	ttl          time.Duration
	maxStaleness time.Duration
	httpClient   *http.Client
	now          func() time.Time

	mu                  sync.Mutex
	keys                map[string]*rsa.PublicKey
	fetchedAt           time.Time // last SUCCESSFUL fetch
	lastAttempt         time.Time // last fetch ATTEMPT, success or failure
	consecutiveFailures int
	staleAlerted        bool
	unknownKidRetryAt   map[string]time.Time // per-kid, not global — see reserveUnknownKidRetry
}

// defaultJWKSClientTimeout backstops a hung fetch (a connection that
// accepts but never responds — not a hard refusal, which fails fast on
// its own) when no context deadline is shorter. Distinct from and
// independent of a per-request context deadline: this fires even for a
// call that somehow reaches fetch() with no deadline on its context at
// all, which the http.Client.Timeout field guards against regardless of
// context (Nolan Reyes, PR #30 review — the shipped wiring passed nil
// for httpClient, resolving to http.DefaultClient, whose zero-value
// Timeout means "none").
const defaultJWKSClientTimeout = 10 * time.Second

// NewJWKSCache constructs a cache fetching from url, refetching at most
// once per ttl (and, for an unrecognized kid specifically, at most once
// per jwksUnknownKidRefetchInterval outside the normal TTL cycle). A nil
// httpClient constructs one with defaultJWKSClientTimeout — never
// http.DefaultClient, whose Timeout is unset (no bound at all) by
// default. maxStaleness is jwksMaxStalenessMultiple * ttl.
func NewJWKSCache(url string, ttl time.Duration, httpClient *http.Client) *JWKSCache {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultJWKSClientTimeout}
	}
	return &JWKSCache{
		url:          url,
		ttl:          ttl,
		maxStaleness: jwksMaxStalenessMultiple * ttl,
		httpClient:   httpClient,
		now:          time.Now,
	}
}

// KeyForKid never holds c.mu across a network fetch (Nolan Reyes, PR #30
// review, finding #2): the prior shape held the lock for the whole
// function body, so one hung fetch stalled every other concurrent
// request through this same cache instance — i.e. every authenticated
// request the verifying Deployment was serving, not just the one that
// triggered the refresh. Concurrent callers that both decide a refresh
// is due may both fetch; that's cheap, occasional duplicate work, and far
// preferable to serializing the whole authenticated surface behind one
// goroutine's network call.
func (c *JWKSCache) KeyForKid(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	needsRefresh := c.keys == nil || c.now().Sub(c.fetchedAt) >= c.ttl
	canAttempt := c.now().Sub(c.lastAttempt) >= jwksMinRefreshRetryInterval
	c.mu.Unlock()

	if needsRefresh {
		if canAttempt {
			if err := c.refresh(ctx); err != nil {
				return nil, err
			}
		} else if err := c.enforceStaleness(); err != nil {
			// Backing off from a very recent failed attempt (see
			// jwksMinRefreshRetryInterval) — no new network call this
			// time, but the staleness ceiling must still be enforced
			// against the existing cache regardless of whether this
			// particular call was the one that tried to refresh it.
			return nil, err
		}
	}

	if key, ok := c.lookupKey(kid); ok {
		return key, nil
	}

	// Unknown kid: might be a key the issuer rotated in since the last
	// TTL-driven refresh. One rate-limited out-of-band refetch before
	// giving up, so a freshly-rotated key isn't rejected for up to a
	// full TTL (Tomasz Wrede, 02 cold review). Best-effort: its error is
	// deliberately ignored here — the outcome that matters is whether
	// the retry populated the kid we're actually looking for, and a
	// refresh() failure here has already been handled (logged / staleness
	// tracked) inside refresh() itself.
	if c.reserveUnknownKidRetry(kid) {
		_ = c.refresh(ctx)
		if key, ok := c.lookupKey(kid); ok {
			return key, nil
		}
	}

	return nil, fmt.Errorf("api: no key found for kid %q", kid)
}

func (c *JWKSCache) lookupKey(kid string) (*rsa.PublicKey, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k, ok := c.keys[kid]
	return k, ok
}

// reserveUnknownKidRetry reports whether an unknown-kid-triggered refetch
// may proceed now for THIS specific kid, and if so atomically marks one
// as just having started. Keyed per kid, not by one global timestamp
// (Ingrid Solano, 02 cold re-check): KeyForKid runs before signature
// verification, so an unauthenticated caller can send a token with a
// fabricated kid at no cost — a single global gate lets that caller
// permanently occupy the one reservation slot by repeating it every
// jwksUnknownKidRefetchInterval, pushing a legitimately-rotated real
// key's own refetch back to the full calendar-TTL rollover, silently
// recreating most of the original gap this mechanism exists to close.
// Per-kid closes that: a garbage kid's repeated reservation never blocks
// a DIFFERENT (real) kid's own reservation. jwksMaxTrackedUnknownKids
// caps how many distinct kids can hold a reservation at once, so a flood
// of distinct fabricated kids still can't hammer the issuer past that
// bound — rate-limiting is scoped per-kid, not removed.
func (c *JWKSCache) reserveUnknownKidRetry(kid string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unknownKidRetryAt == nil {
		c.unknownKidRetryAt = make(map[string]time.Time)
	}
	if last, ok := c.unknownKidRetryAt[kid]; ok && c.now().Sub(last) < jwksUnknownKidRefetchInterval {
		return false
	}

	// Sweep expired entries before checking the size cap, so a kid
	// that's aged out of its own window doesn't count against another
	// kid's chance to reserve.
	for k, t := range c.unknownKidRetryAt {
		if c.now().Sub(t) >= jwksUnknownKidRefetchInterval {
			delete(c.unknownKidRetryAt, k)
		}
	}
	if len(c.unknownKidRetryAt) >= jwksMaxTrackedUnknownKids {
		// At capacity with distinct in-flight-window kids — deny this
		// one rather than let the map grow further. The ordinary
		// TTL-driven refresh still eventually picks up a real rotated
		// key regardless of this outcome.
		return false
	}
	c.unknownKidRetryAt[kid] = c.now()
	return true
}

// refresh performs the network fetch (no lock held during it) and
// updates cache state. Returns an error only when the caller must fail
// the whole verification: no cache exists yet (cold start against an
// unreachable issuer), or the existing cache has exceeded maxStaleness —
// wrapped in ErrJWKSStale in the latter case specifically, so the
// middleware can log AuditJWKSUnavailable rather than an ordinary auth
// failure. A transient failure within the staleness bound returns nil:
// the stale cache is still good enough to use, per the resilience this
// mechanism exists for.
func (c *JWKSCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	c.lastAttempt = c.now()
	c.mu.Unlock()

	fresh, err := c.fetch(ctx)

	c.mu.Lock()
	if err == nil {
		c.keys = fresh
		c.fetchedAt = c.now()
		c.consecutiveFailures = 0
		c.staleAlerted = false
		c.mu.Unlock()
		return nil
	}
	c.consecutiveFailures++
	noCache := c.keys == nil
	c.mu.Unlock()

	if noCache {
		return err
	}
	return c.enforceStaleness()
}

// enforceStaleness checks the existing cache against maxStaleness and
// fails closed (ErrJWKSStale) if it's been exceeded — factored out of
// refresh() so the same check applies on a call that skips an actual
// network attempt because jwksMinRefreshRetryInterval hasn't elapsed yet
// (Oren Castellan, PR #30 re-review: without this, backing off from
// hammering a struggling issuer would also silently suspend the
// staleness ceiling for however long the backoff lasts).
func (c *JWKSCache) enforceStaleness() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keys == nil {
		return errors.New("api: JWKS cache not yet populated")
	}
	staleSince := c.now().Sub(c.fetchedAt)
	if staleSince > c.maxStaleness {
		if !c.staleAlerted {
			c.staleAlerted = true
			// Log line for 04's alerting pipeline on a prolonged
			// fetch-failure run — not a policy decision (AuditLogger is
			// for those), an operational signal that this process's own
			// key infrastructure is degraded.
			log.Printf("api: JWKS unrefreshable for %s across %d consecutive failures (exceeds max staleness %s) — failing closed",
				staleSince, c.consecutiveFailures, c.maxStaleness)
		}
		return fmt.Errorf("%w: stale for %s across %d consecutive failures", ErrJWKSStale, staleSince, c.consecutiveFailures)
	}
	return nil
}

func (c *JWKSCache) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("api: building JWKS request for %s: %w", c.url, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api: fetching JWKS from %s: %w", c.url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api: fetching JWKS from %s: status %d", c.url, resp.StatusCode)
	}
	// 1MiB ceiling — a JWKS response is a handful of RSA keys, never
	// this large; capped so a misbehaving/compromised issuer endpoint
	// can't force this process to buffer an unbounded response body.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("api: reading JWKS response: %w", err)
	}
	return parseJWKS(body)
}
