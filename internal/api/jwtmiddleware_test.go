package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://auth.loginid-takehome.internal"
	testAudience = "loginid-api-service"
)

// fakeKeySource implements KeySource with an in-memory map, for tests
// that don't need real JWKS-over-HTTP fetching (that's jwks_test.go's
// job).
type fakeKeySource struct {
	keys map[string]*rsa.PublicKey
}

func (f *fakeKeySource) KeyForKid(kid string) (*rsa.PublicKey, error) {
	k, ok := f.keys[kid]
	if !ok {
		return nil, errNoKey
	}
	return k, nil
}

var errNoKey = jwt.ErrTokenUnverifiable

func generateRSAKeyPair(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	return key
}

type tokenOpts struct {
	kid      string
	alg      jwt.SigningMethod
	issuer   string
	audience string
	subject  string
	scope    string
	expired  bool
	notYet   bool
}

func signToken(t *testing.T, key *rsa.PrivateKey, opts tokenOpts) string {
	t.Helper()
	now := time.Now()
	exp := now.Add(10 * time.Minute)
	if opts.expired {
		exp = now.Add(-1 * time.Minute)
	}
	iat := now
	nbf := now
	if opts.notYet {
		nbf = now.Add(1 * time.Hour)
	}
	c := claims{
		Scope: opts.scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    opts.issuer,
			Subject:   opts.subject,
			Audience:  jwt.ClaimStrings{opts.audience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(iat),
			NotBefore: jwt.NewNumericDate(nbf),
		},
	}
	token := jwt.NewWithClaims(opts.alg, c)
	token.Header["kid"] = opts.kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

func defaultOpts(kid string) tokenOpts {
	return tokenOpts{
		kid:      kid,
		alg:      jwt.SigningMethodRS256,
		issuer:   testIssuer,
		audience: testAudience,
		subject:  "client-under-test",
		scope:    string(ScopeSearch),
	}
}

func newTestMiddleware(keys map[string]*rsa.PublicKey) func(http.Handler) http.Handler {
	return NewJWTMiddleware(&fakeKeySource{keys: keys}, testIssuer, testAudience)
}

func okHandler(gotCtx *AuthContext) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ac, ok := AuthContextFromContext(r.Context()); ok {
			*gotCtx = ac
		}
		w.WriteHeader(http.StatusOK)
	})
}

func doRequest(mw func(http.Handler) http.Handler, token string) (*httptest.ResponseRecorder, AuthContext) {
	var gotCtx AuthContext
	h := mw(okHandler(&gotCtx))
	req := httptest.NewRequest(http.MethodGet, "/profiles/search", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w, gotCtx
}

func TestJWTMiddleware_ValidTokenPopulatesAuthContext(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	tok := signToken(t, key, defaultOpts("key-1"))

	w, ac := doRequest(mw, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ac.Sub != "client-under-test" || ac.Scope != ScopeSearch {
		t.Errorf("AuthContext = %+v, want Sub=client-under-test Scope=profile:search", ac)
	}
}

func TestJWTMiddleware_MissingToken_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	w, _ := doRequest(mw, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestJWTMiddleware_ExpiredToken_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	opts := defaultOpts("key-1")
	opts.expired = true
	tok := signToken(t, key, opts)

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an expired token", w.Code)
	}
}

func TestJWTMiddleware_WrongAudience_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	opts := defaultOpts("key-1")
	opts.audience = "some-other-service"
	tok := signToken(t, key, opts)

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a mismatched audience", w.Code)
	}
}

func TestJWTMiddleware_WrongIssuer_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	opts := defaultOpts("key-1")
	opts.issuer = "https://not-the-real-issuer.example"
	tok := signToken(t, key, opts)

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a mismatched issuer", w.Code)
	}
}

// TestJWTMiddleware_AlgNone_401 is RFC 8725 §3.1's classic attack: a
// token whose header claims alg:none, carrying no signature at all. The
// algorithm-allowlist passed to jwt.ParseWithClaims must reject this
// before ever reaching the keyfunc/signature-verification step.
func TestJWTMiddleware_AlgNone_401(t *testing.T) {
	mw := newTestMiddleware(map[string]*rsa.PublicKey{})

	unsignedClaims := claims{
		Scope: string(ScopeSearch),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   "attacker",
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, unsignedClaims)
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("constructing alg:none token: %v", err)
	}

	w, _ := doRequest(mw, signed)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an alg:none token", w.Code)
	}
}

// TestJWTMiddleware_HMACConfusionWithPublicKeyAsSecret_401 is RFC 8725
// §3.1's other classic attack: an attacker who knows the RSA public key
// (public by definition — it's published as JWKS) signs a token with
// HS256 using the PEM-encoded public key bytes as the HMAC secret. A
// verifier that blindly uses whatever alg the token header claims, and
// hands the "key" returned by its keyfunc to that algorithm's verifier,
// gets fooled: HS256 with the public key as the secret "verifies"
// successfully against a keyfunc that returns the RSA public key
// unconditionally. The algorithm allowlist must reject this the same way
// it rejects alg:none.
func TestJWTMiddleware_HMACConfusionWithPublicKeyAsSecret_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})

	pubDER := x509.MarshalPKCS1PublicKey(&key.PublicKey)

	forged := claims{
		Scope: string(ScopeSearch),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   "attacker",
			Audience:  jwt.ClaimStrings{testAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, forged)
	tok.Header["kid"] = "key-1"
	signed, err := tok.SignedString(pubDER)
	if err != nil {
		t.Fatalf("signing HMAC-confusion token: %v", err)
	}

	w, _ := doRequest(mw, signed)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an HS256-with-public-key-as-secret forged token", w.Code)
	}
}

// TestJWTMiddleware_KidRotation_BothKeysVerify is the acceptance
// criterion for key rotation: two tokens signed under two different kids
// both verify successfully when both keys are configured, supporting a
// rotation overlap window.
func TestJWTMiddleware_KidRotation_BothKeysVerify(t *testing.T) {
	oldKey := generateRSAKeyPair(t)
	newKey := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{
		"key-old": &oldKey.PublicKey,
		"key-new": &newKey.PublicKey,
	})

	oldTok := signToken(t, oldKey, defaultOpts("key-old"))
	newTok := signToken(t, newKey, defaultOpts("key-new"))

	if w, _ := doRequest(mw, oldTok); w.Code != http.StatusOK {
		t.Errorf("old-kid token status = %d, want 200 during rotation overlap", w.Code)
	}
	if w, _ := doRequest(mw, newTok); w.Code != http.StatusOK {
		t.Errorf("new-kid token status = %d, want 200 during rotation overlap", w.Code)
	}
}

func TestJWTMiddleware_UnknownKid_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	tok := signToken(t, key, defaultOpts("key-does-not-exist"))

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for an unrecognized kid", w.Code)
	}
}

func TestJWTMiddleware_WrongKeyForKid_401(t *testing.T) {
	signingKey := generateRSAKeyPair(t)
	otherKey := generateRSAKeyPair(t)
	// The keyfunc returns otherKey's public key for "key-1" — simulating
	// a mismatched/corrupted JWKS entry. Signature verification must fail.
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &otherKey.PublicKey})
	tok := signToken(t, signingKey, defaultOpts("key-1"))

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 when the signature doesn't verify against the returned key", w.Code)
	}
}

func TestJWTMiddleware_NoScopeClaim_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	opts := defaultOpts("key-1")
	opts.scope = ""
	tok := signToken(t, key, opts)

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a token with no recognized scope", w.Code)
	}
}

func TestJWTMiddleware_MultipleScopes_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	opts := defaultOpts("key-1")
	opts.scope = string(ScopeSearch) + " " + string(ScopeReadOwn)
	tok := signToken(t, key, opts)

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — this verifier does not disambiguate a multi-scope token per request", w.Code)
	}
}

func TestJWTMiddleware_NoSubClaim_401(t *testing.T) {
	key := generateRSAKeyPair(t)
	mw := newTestMiddleware(map[string]*rsa.PublicKey{"key-1": &key.PublicKey})
	opts := defaultOpts("key-1")
	opts.subject = ""
	tok := signToken(t, key, opts)

	w, _ := doRequest(mw, tok)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a token with no sub claim", w.Code)
	}
}

func TestDeadlineMiddleware_AppliesDeadline(t *testing.T) {
	var sawDeadline bool
	h := NewDeadlineMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawDeadline = r.Context().Deadline()
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !sawDeadline {
		t.Error("request context should carry a deadline after NewDeadlineMiddleware")
	}
}
