package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func jwkFromPublicKey(kid string, pub *rsa.PublicKey) jwk {
	return jwk{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(bigEndianExponent(pub.E)),
	}
}

func bigEndianExponent(e int) []byte {
	// Standard RSA public exponent (65537) fits in 3 bytes; encode
	// minimally, matching how real JWKS producers encode "e".
	b := []byte{byte(e >> 16), byte(e >> 8), byte(e)}
	i := 0
	for i < len(b)-1 && b[i] == 0 {
		i++
	}
	return b[i:]
}

func jwksBody(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
	t.Helper()
	doc := jwksResponse{Keys: []jwk{jwkFromPublicKey(kid, pub)}}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}
	return body
}

func TestParseJWKS_RoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	data := jwksBody(t, "key-1", &key.PublicKey)

	keys, err := parseJWKS(data)
	if err != nil {
		t.Fatalf("parseJWKS: %v", err)
	}
	got, ok := keys["key-1"]
	if !ok {
		t.Fatal("parsed key set missing key-1")
	}
	if got.N.Cmp(key.N) != 0 || got.E != key.E {
		t.Error("parsed public key does not match the original")
	}
}

func TestParseJWKS_SkipsNonRSAKeys(t *testing.T) {
	data := []byte(`{"keys":[{"kty":"EC","kid":"ec-1","crv":"P-256","x":"AAAA","y":"AAAA"}]}`)
	keys, err := parseJWKS(data)
	if err != nil {
		t.Fatalf("parseJWKS: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected a non-RSA key to be skipped, got %d keys", len(keys))
	}
}

func TestParseJWKS_MissingKidIsError(t *testing.T) {
	data := []byte(`{"keys":[{"kty":"RSA","n":"AAAA","e":"AQAB"}]}`)
	if _, err := parseJWKS(data); err == nil {
		t.Error("expected an error for an RSA key with no kid")
	}
}

// jwksTestServer serves body while up, or a configurable failure while
// down — atomic so it's safe to flip from the test goroutine while the
// cache's own fetches run (any of them) against it.
type jwksTestServer struct {
	srv  *httptest.Server
	up   atomic.Bool
	body []byte
}

func newJWKSTestServer(body []byte) *jwksTestServer {
	s := &jwksTestServer{body: body}
	s.up.Store(true)
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.up.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(s.body)
	}))
	return s
}

func (s *jwksTestServer) Close() { s.srv.Close() }

func TestJWKSCache_FetchesAndCaches(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	var requests atomic.Int32
	body := jwksBody(t, "key-1", &key.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	ctx := context.Background()
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("second KeyForKid: %v", err)
	}
	if requests.Load() != 1 {
		t.Errorf("fetched the JWKS endpoint %d times, want exactly 1 (cached within TTL)", requests.Load())
	}
}

func TestJWKSCache_RefetchesAfterTTLExpires(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	var requests atomic.Int32
	body := jwksBody(t, "key-1", &key.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Minute, nil)
	fakeNow := time.Now()
	cache.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	fakeNow = fakeNow.Add(time.Minute + time.Second)
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("second KeyForKid: %v", err)
	}
	if requests.Load() < 2 {
		t.Errorf("fetched the JWKS endpoint %d times, want at least 2 (TTL should have expired)", requests.Load())
	}
}

func TestJWKSCache_UnknownKidWithNoStaleCacheIsError(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	body := jwksBody(t, "key-1", &key.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	if _, err := cache.KeyForKid(context.Background(), "no-such-kid"); err == nil {
		t.Error("expected an error for an unrecognized kid")
	}
}

// TestJWKSCache_StaleCacheSurvivesTransientFetchFailure covers the
// resilience case within the staleness bound: a fetch failure well
// inside maxStaleness still serves the last-good key set.
func TestJWKSCache_StaleCacheSurvivesTransientFetchFailure(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	body := jwksBody(t, "key-1", &key.PublicKey)
	s := newJWKSTestServer(body)
	defer s.Close()

	cache := NewJWKSCache(s.srv.URL, time.Minute, nil) // maxStaleness = 3*ttl = 3min
	fakeNow := time.Now()
	cache.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	fakeNow = fakeNow.Add(time.Minute + time.Second) // TTL expired, well within the 3min staleness bound
	s.up.Store(false)
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Errorf("KeyForKid during a transient fetch failure = %v, want the stale cache to still serve the key", err)
	}
}

// TestJWKSCache_FailsClosedPastMaxStaleness is the PM's ruling on Tomasz
// Wrede's HIGH finding: a signing key can be revoked because it's
// compromised, and if the JWKS endpoint is unreachable during that same
// incident, an unbounded stale-cache fallback would keep verifying
// tokens signed by the revoked key for as long as fetches keep failing.
// Past maxStaleness (3x ttl, per the PM's ruling), the cache must fail
// closed — ErrJWKSStale — rather than serve the stale set indefinitely.
func TestJWKSCache_FailsClosedPastMaxStaleness(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	body := jwksBody(t, "key-1", &key.PublicKey)
	s := newJWKSTestServer(body)
	defer s.Close()

	cache := NewJWKSCache(s.srv.URL, time.Minute, nil) // maxStaleness = 3min
	fakeNow := time.Now()
	cache.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	s.up.Store(false)
	// Advance past maxStaleness (3min) from the last successful fetch.
	fakeNow = fakeNow.Add(4 * time.Minute)
	_, err = cache.KeyForKid(ctx, "key-1")
	if err == nil {
		t.Fatal("expected an error once the cache exceeds max staleness with the issuer still unreachable")
	}
	if !errors.Is(err, ErrJWKSStale) {
		t.Errorf("error = %v, want it to wrap ErrJWKSStale so the middleware can log a distinct audit event", err)
	}
}

// TestJWKSCache_UnknownKidTriggersOneRateLimitedRefetch is the PM's
// ruling on Tomasz Wrede's MEDIUM-HIGH finding: an unknown kid (a key
// the issuer may have just rotated in) triggers one out-of-band refetch
// before the cache gives up, rather than waiting up to a full TTL for
// the next calendar-scheduled refresh.
func TestJWKSCache_UnknownKidTriggersOneRateLimitedRefetch(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	var requests atomic.Int32
	var serveNewKey atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if serveNewKey.Load() {
			_, _ = w.Write(jwksBody(t, "key-new", &newKey.PublicKey))
		} else {
			_, _ = w.Write(jwksBody(t, "key-old", &oldKey.PublicKey))
		}
	}))
	defer srv.Close()

	// A long TTL so the calendar-scheduled refresh would NOT fire on its
	// own within this test — proving any successful lookup of "key-new"
	// came from the unknown-kid-triggered refetch, not an ordinary TTL
	// rollover.
	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-old"); err != nil {
		t.Fatalf("priming the cache with key-old: %v", err)
	}
	requestsAfterPriming := requests.Load()

	// The issuer rotates: key-new is now published, but the cache's TTL
	// hasn't expired.
	serveNewKey.Store(true)
	if _, err := cache.KeyForKid(ctx, "key-new"); err != nil {
		t.Fatalf("KeyForKid(key-new) = %v, want the unknown-kid refetch to find it", err)
	}
	if requests.Load() <= requestsAfterPriming {
		t.Error("expected an extra fetch triggered by the unknown kid, beyond the priming fetch")
	}
}

func TestJWKSCache_UnknownKidRefetchIsRateLimited(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	var requests atomic.Int32
	body := jwksBody(t, "key-1", &key.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	fakeNow := time.Now()
	cache.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("priming: %v", err)
	}
	requestsAfterPriming := requests.Load()

	// Two lookups for an unknown kid in quick succession (same fake
	// "now") must trigger at most one extra refetch between them, not
	// one per lookup — that's the rate limit closing the "unknown-kid
	// flood hammers the issuer" gap.
	_, _ = cache.KeyForKid(ctx, "no-such-kid")
	afterFirstUnknown := requests.Load()
	_, _ = cache.KeyForKid(ctx, "no-such-kid")
	afterSecondUnknown := requests.Load()

	if afterFirstUnknown != requestsAfterPriming+1 {
		t.Errorf("first unknown-kid lookup triggered %d extra fetch(es), want exactly 1", afterFirstUnknown-requestsAfterPriming)
	}
	if afterSecondUnknown != afterFirstUnknown {
		t.Errorf("second unknown-kid lookup (within the rate-limit window) triggered an extra fetch: %d -> %d, want no change", afterFirstUnknown, afterSecondUnknown)
	}
}

// TestJWKSCache_FailedRefreshDoesNotHammerIssuer is Oren Castellan's PR
// #30 re-review finding: without a floor on retry attempts, every single
// request during a sustained issuer outage triggered its own network
// fetch (bounded only by maxStaleness, not by request volume) — the same
// "don't hammer a struggling dependency" reasoning already applied to the
// unknown-kid path, missing from the ordinary TTL-refresh path. Multiple
// KeyForKid calls in quick succession, all past TTL with the issuer down,
// must make only one real fetch attempt per jwksMinRefreshRetryInterval.
func TestJWKSCache_FailedRefreshDoesNotHammerIssuer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	body := jwksBody(t, "key-1", &key.PublicKey)
	s := newJWKSTestServer(body)
	defer s.Close()

	cache := NewJWKSCache(s.srv.URL, time.Minute, nil) // maxStaleness = 3min, min retry floor = 5s
	fakeNow := time.Now()
	cache.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("priming: %v", err)
	}

	s.up.Store(false)
	fakeNow = fakeNow.Add(time.Minute + time.Second) // TTL expired

	var requestsBefore int32
	// Read the server's own request counter via a second server field
	// would require plumbing; instead assert indirectly: repeated calls
	// within the retry floor must all return the SAME error quickly
	// (served from enforceStaleness, not a fresh fetch attempt) — a real
	// per-call fetch would each take however long the (down) server's
	// connection refusal takes, which is fast here, so we assert on
	// lastAttempt not advancing instead.
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("first post-TTL call: %v", err)
	}
	cache.mu.Lock()
	firstAttempt := cache.lastAttempt
	cache.mu.Unlock()
	_ = requestsBefore

	fakeNow = fakeNow.Add(time.Second) // still within jwksMinRefreshRetryInterval (5s) of firstAttempt
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("second post-TTL call (within retry floor): %v", err)
	}
	cache.mu.Lock()
	secondAttempt := cache.lastAttempt
	cache.mu.Unlock()

	if !secondAttempt.Equal(firstAttempt) {
		t.Errorf("a second call within jwksMinRefreshRetryInterval triggered a new fetch attempt (lastAttempt moved from %v to %v) — want it to back off and reuse the stale cache instead", firstAttempt, secondAttempt)
	}

	// Past the retry floor, a new attempt IS made.
	fakeNow = fakeNow.Add(jwksMinRefreshRetryInterval)
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("third post-TTL call (past retry floor): %v", err)
	}
	cache.mu.Lock()
	thirdAttempt := cache.lastAttempt
	cache.mu.Unlock()
	if thirdAttempt.Equal(firstAttempt) {
		t.Error("a call past jwksMinRefreshRetryInterval should have made a new fetch attempt")
	}
}

// TestJWKSCache_StalenessEnforcedEvenWhenBackingOffFromRetry proves the
// staleness ceiling isn't silently suspended while the retry floor is
// skipping real fetch attempts — enforceStaleness must still fire once
// maxStaleness is exceeded, even on a call that isn't itself attempting
// a fetch.
func TestJWKSCache_StalenessEnforcedEvenWhenBackingOffFromRetry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	body := jwksBody(t, "key-1", &key.PublicKey)
	s := newJWKSTestServer(body)
	defer s.Close()

	cache := NewJWKSCache(s.srv.URL, time.Minute, nil) // maxStaleness = 3min
	fakeNow := time.Now()
	cache.now = func() time.Time { return fakeNow }
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("priming: %v", err)
	}
	s.up.Store(false)

	// One real attempt right after TTL expires, which fails and sets
	// lastAttempt.
	fakeNow = fakeNow.Add(time.Minute + time.Second)
	_, _ = cache.KeyForKid(ctx, "key-1")

	// Jump straight past maxStaleness (3min from the last SUCCESSFUL
	// fetch), but still within jwksMinRefreshRetryInterval of the last
	// ATTEMPT — this call must back off from a new fetch attempt and
	// still correctly fail closed via enforceStaleness, not silently
	// succeed with the stale cache because no new attempt was made.
	fakeNow = fakeNow.Add(4 * time.Minute)
	_, err = cache.KeyForKid(ctx, "key-1")
	if err == nil {
		t.Fatal("expected an error once past max staleness, even on a call that backs off from a new fetch attempt")
	}
	if !errors.Is(err, ErrJWKSStale) {
		t.Errorf("error = %v, want ErrJWKSStale", err)
	}
}

// TestJWKSCache_UnknownKidReservationIsPerKidNotGlobal is Ingrid
// Solano's (02) cold re-check finding: KeyForKid runs before signature
// verification, so an unauthenticated caller can send a token with a
// fabricated kid at no cost. A single GLOBAL reservation gate would let
// that caller occupy the one slot by repeating a garbage kid every
// jwksUnknownKidRefetchInterval, starving a DIFFERENT, legitimately
// rotated-in real kid's own refetch back to the full calendar TTL —
// silently recreating most of the original gap. Reserving per kid closes
// this: a garbage kid's own reservation must not block a different real
// kid's reservation in the same window.
func TestJWKSCache_UnknownKidReservationIsPerKidNotGlobal(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	var serveNewKey atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if serveNewKey.Load() {
			_, _ = w.Write(jwksBody(t, "key-new", &newKey.PublicKey))
		} else {
			_, _ = w.Write(jwksBody(t, "key-old", &oldKey.PublicKey))
		}
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	ctx := context.Background()

	if _, err := cache.KeyForKid(ctx, "key-old"); err != nil {
		t.Fatalf("priming: %v", err)
	}

	// Attacker occupies the reservation for a garbage kid.
	_, _ = cache.KeyForKid(ctx, "garbage-kid-from-attacker")
	// Repeats it immediately — this must be denied (rate-limited), same
	// as before the fix.
	_, _ = cache.KeyForKid(ctx, "garbage-kid-from-attacker")

	// The issuer rotates for real. A legitimate lookup for the new kid,
	// in the SAME window the attacker's garbage kid already consumed,
	// must still get its own reservation and succeed — not be starved by
	// the attacker's unrelated kid.
	serveNewKey.Store(true)
	if _, err := cache.KeyForKid(ctx, "key-new"); err != nil {
		t.Errorf("KeyForKid(key-new) = %v, want the real rotated key's own reservation to succeed regardless of an attacker's unrelated garbage-kid reservation", err)
	}
}

// TestJWKSCache_UnknownKidReservationCapsDistinctKids is the other half
// of Ingrid's ruling: rate-limiting is scoped per kid, not removed — a
// flood of distinct fabricated kids must still be bounded, not each get
// their own unconditional extra fetch.
func TestJWKSCache_UnknownKidReservationCapsDistinctKids(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	var requests atomic.Int32
	body := jwksBody(t, "key-1", &key.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	ctx := context.Background()
	if _, err := cache.KeyForKid(ctx, "key-1"); err != nil {
		t.Fatalf("priming: %v", err)
	}
	requestsAfterPriming := requests.Load()

	// Flood with more distinct garbage kids than jwksMaxTrackedUnknownKids.
	for i := 0; i < jwksMaxTrackedUnknownKids+20; i++ {
		_, _ = cache.KeyForKid(ctx, fmt.Sprintf("garbage-kid-%d", i))
	}

	extraFetches := requests.Load() - requestsAfterPriming
	if int(extraFetches) > jwksMaxTrackedUnknownKids {
		t.Errorf("a flood of %d distinct garbage kids triggered %d extra fetches, want at most %d (the cap)", jwksMaxTrackedUnknownKids+20, extraFetches, jwksMaxTrackedUnknownKids)
	}
}
