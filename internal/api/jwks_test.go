package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestParseJWKS_RoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	doc := jwksResponse{Keys: []jwk{jwkFromPublicKey("key-1", &key.PublicKey)}}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}

	keys, err := parseJWKS(data)
	if err != nil {
		t.Fatalf("parseJWKS: %v", err)
	}
	got, ok := keys["key-1"]
	if !ok {
		t.Fatal("parsed key set missing key-1")
	}
	if got.N.Cmp(key.PublicKey.N) != 0 || got.E != key.PublicKey.E {
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

func TestJWKSCache_FetchesAndCaches(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	doc := jwksResponse{Keys: []jwk{jwkFromPublicKey("key-1", &key.PublicKey)}}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	if _, err := cache.KeyForKid("key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	if _, err := cache.KeyForKid("key-1"); err != nil {
		t.Fatalf("second KeyForKid: %v", err)
	}
	if requests != 1 {
		t.Errorf("fetched the JWKS endpoint %d times, want exactly 1 (cached within TTL)", requests)
	}
}

func TestJWKSCache_RefetchesAfterTTLExpires(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	doc := jwksResponse{Keys: []jwk{jwkFromPublicKey("key-1", &key.PublicKey)}}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Millisecond, nil)
	if _, err := cache.KeyForKid("key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := cache.KeyForKid("key-1"); err != nil {
		t.Fatalf("second KeyForKid: %v", err)
	}
	if requests < 2 {
		t.Errorf("fetched the JWKS endpoint %d times, want at least 2 (TTL should have expired)", requests)
	}
}

func TestJWKSCache_UnknownKidIsError(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	doc := jwksResponse{Keys: []jwk{jwkFromPublicKey("key-1", &key.PublicKey)}}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Hour, nil)
	if _, err := cache.KeyForKid("no-such-kid"); err == nil {
		t.Error("expected an error for an unrecognized kid")
	}
}

func TestJWKSCache_StaleCacheSurvivesTransientFetchFailure(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	doc := jwksResponse{Keys: []jwk{jwkFromPublicKey("key-1", &key.PublicKey)}}
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshaling JWKS: %v", err)
	}

	failing := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cache := NewJWKSCache(srv.URL, time.Millisecond, nil)
	if _, err := cache.KeyForKid("key-1"); err != nil {
		t.Fatalf("first KeyForKid: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	failing = true
	if _, err := cache.KeyForKid("key-1"); err != nil {
		t.Errorf("KeyForKid during a transient fetch failure = %v, want the stale cache to still serve the key", err)
	}
}
