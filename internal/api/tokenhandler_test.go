package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// fakeClientStore implements ClientStore, per decisions/test-double-strategy.md.
type fakeClientStore struct {
	records map[string]ClientRecord
	getErr  error
	gotIDs  []string // records every clientID Get was called with, for call-shape assertions
}

func (f *fakeClientStore) Get(_ context.Context, clientID string) (ClientRecord, bool, error) {
	f.gotIDs = append(f.gotIDs, clientID)
	if f.getErr != nil {
		return ClientRecord{}, false, f.getErr
	}
	rec, ok := f.records[clientID]
	return rec, ok, nil
}

// fakeGrantLimiter implements GrantLimiter.
type fakeGrantLimiter struct {
	allow           bool
	allowErr        error
	failedClientIDs []string
	succeededIDs    []string
}

func (f *fakeGrantLimiter) Allow(_ context.Context, clientID, sourceIP string) (bool, error) {
	return f.allow, f.allowErr
}
func (f *fakeGrantLimiter) RecordFailure(_ context.Context, clientID, sourceIP string) {
	f.failedClientIDs = append(f.failedClientIDs, clientID)
}
func (f *fakeGrantLimiter) RecordSuccess(_ context.Context, clientID, sourceIP string) {
	f.succeededIDs = append(f.succeededIDs, clientID)
}

func testSigningKey(t *testing.T) *SigningKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	return &SigningKey{Private: priv, Kid: "test-kid"}
}

func postToken(t *testing.T, h http.HandlerFunc, form url.Values, clientID, clientSecret string, setBasicAuth bool) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if setBasicAuth {
		req.SetBasicAuth(clientID, clientSecret)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Result()
}

func decodeTokenError(t *testing.T, resp *http.Response) tokenErrorResponse {
	t.Helper()
	var body tokenErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	return body
}

func TestNewTokenHandler_MissingGrantType_UnsupportedGrantType(t *testing.T) {
	store := &fakeClientStore{records: map[string]ClientRecord{}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	resp := postToken(t, h, url.Values{}, "any", "any", true)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if body := decodeTokenError(t, resp); body.Error != "unsupported_grant_type" {
		t.Errorf("error = %q, want unsupported_grant_type", body.Error)
	}
}

func TestNewTokenHandler_NoBasicAuth_InvalidClient401(t *testing.T) {
	store := &fakeClientStore{records: map[string]ClientRecord{}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "", "", false)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Basic") {
		t.Errorf("WWW-Authenticate = %q, want a Basic challenge", got)
	}
	if len(store.gotIDs) != 0 {
		t.Errorf("store.Get called %d times, want 0 — no client_id exists yet to key by", len(store.gotIDs))
	}
}

// TestNewTokenHandler_UnknownClient_And_WrongSecret_ByteIdenticalResponses
// is RFC 6749 §5.2's own requirement plus Tobias Lindqvist's timing-oracle
// objection (LT-51 refinement): an unknown client_id and a wrong secret
// for a known client_id must be indistinguishable to the caller — same
// status, same body — and internally must run the same real Argon2id
// comparison exactly once, never short-circuiting one path cheaper than
// the other.
func TestNewTokenHandler_UnknownClient_And_WrongSecret_ByteIdenticalResponses(t *testing.T) {
	hash, err := hashSecret("correct-secret")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	store := &fakeClientStore{records: map[string]ClientRecord{
		"known-client": {ClientID: "known-client", Name: "Known", ClientSecretHash: hash, GrantedScope: Scope("profile:search"), Audience: "aud", Active: true},
	}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}

	respUnknown := postToken(t, h, form, "does-not-exist", "whatever", true)
	bodyUnknown, _ := io.ReadAll(respUnknown.Body)

	respWrongSecret := postToken(t, h, form, "known-client", "wrong-secret", true)
	bodyWrongSecret, _ := io.ReadAll(respWrongSecret.Body)

	if respUnknown.StatusCode != http.StatusUnauthorized || respWrongSecret.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status codes = %d, %d, want both 401", respUnknown.StatusCode, respWrongSecret.StatusCode)
	}
	if string(bodyUnknown) != string(bodyWrongSecret) {
		t.Errorf("response bodies differ: unknown-client=%q wrong-secret=%q — must be byte-identical", bodyUnknown, bodyWrongSecret)
	}

	if len(limiter.failedClientIDs) != 2 {
		t.Errorf("RecordFailure called %d times, want 2 (once per failed attempt)", len(limiter.failedClientIDs))
	}
}

func TestNewTokenHandler_FoundButNoSecretSet_TreatedSameAsNotFound(t *testing.T) {
	// secret_state = "none"/"revoked" rows have an empty ClientSecretHash
	// (the migration's own CHECK ties the two together) — this must run
	// the dummyHash comparison, not a malformed-hash short-circuit, per
	// the same timing-parity reasoning as the unknown-client_id case.
	store := &fakeClientStore{records: map[string]ClientRecord{
		"provisioned-no-secret": {ClientID: "provisioned-no-secret", Name: "No Secret Yet", ClientSecretHash: "", GrantedScope: Scope("profile:search"), Audience: "aud", Active: true},
	}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "provisioned-no-secret", "anything", true)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if body := decodeTokenError(t, resp); body.Error != "invalid_client" {
		t.Errorf("error = %q, want invalid_client", body.Error)
	}
}

func TestNewTokenHandler_CorrectCredentials_IssuesValidToken(t *testing.T) {
	hash, err := hashSecret("s3cr3t")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	key := testSigningKey(t)
	store := &fakeClientStore{records: map[string]ClientRecord{
		"known-client": {ClientID: "known-client", Name: "Known", ClientSecretHash: hash, GrantedScope: Scope("profile:search"), Audience: "loginid-api-service", Active: true},
	}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, key, "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "known-client", "s3cr3t", true)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store (RFC 6749 §5.1)", got)
	}

	var body tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding success body: %v", err)
	}
	if body.TokenType != "Bearer" {
		t.Errorf("token_type = %q, want Bearer", body.TokenType)
	}
	if body.Scope != "profile:search" {
		t.Errorf("scope = %q, want profile:search", body.Scope)
	}

	parsed, err := jwt.ParseWithClaims(body.AccessToken, &claims{}, func(tok *jwt.Token) (any, error) {
		return &key.Private.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Fatalf("parsing/verifying minted token: %v", err)
	}
	c := parsed.Claims.(*claims)
	if c.Subject != "known-client" {
		t.Errorf("sub = %q, want known-client", c.Subject)
	}
	if c.Issuer != "https://issuer.test" {
		t.Errorf("iss = %q, want https://issuer.test", c.Issuer)
	}
	if len(c.Audience) != 1 || c.Audience[0] != "loginid-api-service" {
		t.Errorf("aud = %v, want [loginid-api-service]", c.Audience)
	}
	if c.Scope != "profile:search" {
		t.Errorf("scope claim = %q, want profile:search", c.Scope)
	}
	if parsed.Header["kid"] != key.Kid {
		t.Errorf("kid header = %v, want %q", parsed.Header["kid"], key.Kid)
	}

	if len(limiter.succeededIDs) != 1 || limiter.succeededIDs[0] != "known-client" {
		t.Errorf("RecordSuccess calls = %v, want exactly one for known-client", limiter.succeededIDs)
	}
}

func TestNewTokenHandler_ScopeMismatch_InvalidScope(t *testing.T) {
	hash, err := hashSecret("s3cr3t")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	store := &fakeClientStore{records: map[string]ClientRecord{
		"known-client": {ClientID: "known-client", Name: "Known", ClientSecretHash: hash, GrantedScope: Scope("profile:search"), Audience: "aud", Active: true},
	}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}, "scope": {"profile:read:any"}}
	resp := postToken(t, h, form, "known-client", "s3cr3t", true)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if body := decodeTokenError(t, resp); body.Error != "invalid_scope" {
		t.Errorf("error = %q, want invalid_scope", body.Error)
	}
}

func TestNewTokenHandler_InactiveClient_InvalidClient401(t *testing.T) {
	hash, err := hashSecret("s3cr3t")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	store := &fakeClientStore{records: map[string]ClientRecord{
		"disabled-client": {ClientID: "disabled-client", Name: "Disabled", ClientSecretHash: hash, GrantedScope: Scope("profile:search"), Audience: "aud", Active: false},
	}}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "disabled-client", "s3cr3t", true)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (disabled status must still reject even with the correct secret)", resp.StatusCode)
	}
}

func TestNewTokenHandler_LimiterDenies_SlowDown429(t *testing.T) {
	store := &fakeClientStore{records: map[string]ClientRecord{}}
	limiter := &fakeGrantLimiter{allow: false}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "whoever", "whatever", true)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if len(store.gotIDs) != 0 {
		t.Errorf("store.Get called %d times, want 0 — the limiter must gate before touching the store", len(store.gotIDs))
	}
}

func TestNewTokenHandler_NilStore_ServerError(t *testing.T) {
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(nil, limiter, testSigningKey(t), "https://issuer.test")

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "whoever", "whatever", true)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil ClientStore must fail closed, not panic)", resp.StatusCode)
	}
	if body := decodeTokenError(t, resp); body.Error != "server_error" {
		t.Errorf("error = %q, want server_error", body.Error)
	}
}
