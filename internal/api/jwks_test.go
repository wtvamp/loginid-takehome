package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
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
