package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"loginid-takehome/internal/config"
	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

// fakeRepo is a minimal dao.Repository — only Profiles().Search is
// exercised by this file's tests; everything else panics, so a test
// that reaches an unexpected method fails loudly rather than silently
// zero-valuing (mirrors internal/api's own hand-written fakes,
// decisions/test-double-strategy.md).
type fakeRepo struct{ profiles fakeProfiles }

func (f *fakeRepo) Profiles() dao.ProfileRepository       { return &f.profiles }
func (f *fakeRepo) Credentials() dao.CredentialRepository { panic("not used") }
func (f *fakeRepo) Methods() dao.AuthMethodRepository     { panic("not used") }
func (f *fakeRepo) CreateProfileWithCredential(ctx context.Context, p *model.UserProfile, c *model.UserCredential) (*model.UserProfile, *model.UserCredential, error) {
	panic("not used")
}
func (f *fakeRepo) DeleteExpired(ctx context.Context, class dao.RetentionClass, olderThan time.Time, maxRows int) (dao.SweepResult, error) {
	panic("not used")
}
func (f *fakeRepo) DeleteProfile(ctx context.Context, id string, externalRef *string) error {
	panic("not used")
}
func (f *fakeRepo) Close() error { return nil }

type fakeProfiles struct{}

func (fakeProfiles) Get(ctx context.Context, id string) (*model.UserProfile, error) {
	return &model.UserProfile{ID: id, Name: "Jane Doe"}, nil
}
func (fakeProfiles) Search(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
	return []model.UserProfile{{ID: "p1", Name: "Jane Doe"}}, 1, nil
}
func (fakeProfiles) Create(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	panic("not used")
}
func (fakeProfiles) Update(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	panic("not used")
}
func (fakeProfiles) Upsert(ctx context.Context, p *model.UserProfile) (*model.UserProfile, error) {
	panic("not used")
}
func (fakeProfiles) Delete(ctx context.Context, id string) error { panic("not used") }

const (
	testIssuer   = "https://auth.loginid-takehome.internal"
	testAudience = "loginid-api-service"
)

func startFakeJWKS(t *testing.T, kid string, pub *rsa.PublicKey) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}), // 65537
		}},
	})
	if err != nil {
		t.Fatalf("marshaling fake JWKS: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, kid, scope string) string {
	t.Helper()
	c := jwt.MapClaims{
		"iss":   testIssuer,
		"aud":   testAudience,
		"sub":   "client-under-test",
		"scope": scope,
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("signing test token: %v", err)
	}
	return signed
}

func TestNewVerifierRouter_UnauthenticatedRequest_401(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, AuthJWTAudience: testAudience, AuthJWKSURL: jwks.URL}
	router := NewVerifierRouter(cfg, &fakeRepo{})

	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 with no bearer token", w.Code)
	}
}

// TestNewVerifierRouter_AuthenticatedSearch_ReachesHandler proves the
// full stack — deadline middleware, JWT verification against a real
// JWKS-over-HTTP fetch, AuthContext population, and LT-39's handler —
// is wired together correctly end to end, not just each piece in
// isolation.
func TestNewVerifierRouter_AuthenticatedSearch_ReachesHandler(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, AuthJWTAudience: testAudience, AuthJWKSURL: jwks.URL}
	router := NewVerifierRouter(cfg, &fakeRepo{})
	tok := signTestToken(t, key, "key-1", "profile:search")

	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"jane"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestNewVerifierRouter_HealthzUnaffectedByAuth(t *testing.T) {
	cfg := config.Config{AuthJWTIssuer: testIssuer, AuthJWTAudience: testAudience, AuthJWKSURL: "http://unused.invalid"}
	router := NewVerifierRouter(cfg, &fakeRepo{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — /healthz must stay unauthenticated per handoff-03-auth.md", w.Code)
	}
}

// TestNewVerifierRouter_ReadOwn_AllowedAndDenied is Marcus Ilori's (02)
// ruling: the router-level test that was missing entirely for
// GET /profiles/{id} under read:own — both the allow and deny paths
// through the real StopgapAuthorizer, wired the way cmd/api-service
// actually wires it (via cfg.AuthzQASub/AuthzQAAllowedProfileID).
func TestNewVerifierRouter_ReadOwn_AllowedAndDenied(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{
		AuthJWTIssuer:           testIssuer,
		AuthJWTAudience:         testAudience,
		AuthJWKSURL:             jwks.URL,
		AuthzQASub:              "client-under-test", // matches signTestToken's hardcoded sub
		AuthzQAAllowedProfileID: "allowed-profile",
	}
	router := NewVerifierRouter(cfg, &fakeRepo{})
	tok := signTestToken(t, key, "key-1", "profile:read:own")

	allowedReq := httptest.NewRequest(http.MethodGet, "/profiles/allowed-profile", nil)
	allowedReq.Header.Set("Authorization", "Bearer "+tok)
	wAllowed := httptest.NewRecorder()
	router.ServeHTTP(wAllowed, allowedReq)
	if wAllowed.Code != http.StatusOK {
		t.Errorf("allowed profile: status = %d, want 200, body=%s", wAllowed.Code, wAllowed.Body.String())
	}

	deniedReq := httptest.NewRequest(http.MethodGet, "/profiles/some-other-profile", nil)
	deniedReq.Header.Set("Authorization", "Bearer "+tok)
	wDenied := httptest.NewRecorder()
	router.ServeHTTP(wDenied, deniedReq)
	if wDenied.Code != http.StatusForbidden {
		t.Errorf("unseeded profile: status = %d, want 403 (StopgapAuthorizer's safe-default deny), body=%s", wDenied.Code, wDenied.Body.String())
	}
}

// TestNewVerifierRouter_NilRepo_ProtectedRoutesFailClosed503 is the PM's
// direct question: on a cluster where the database doesn't exist yet (no
// DB_DRIVER/DB_DSN configured), the verifying Deployment must still come
// up with /healthz green — cmd/api-service/main.go passes repo=nil
// rather than crash-looping — and the two protected routes must fail
// closed (503), not panic on a nil Repository. Auth still runs first: an
// unauthenticated request still gets 401, proven by the companion
// TestNewVerifierRouter_UnauthenticatedRequest_401-shaped assertion
// inline below.
func TestNewVerifierRouter_NilRepo_ProtectedRoutesFailClosed503(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, AuthJWTAudience: testAudience, AuthJWKSURL: jwks.URL}
	router := NewVerifierRouter(cfg, nil) // no DAO repository — the no-database-yet case
	tok := signTestToken(t, key, "key-1", "profile:search")

	// /healthz must still be green.
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	wHealth := httptest.NewRecorder()
	router.ServeHTTP(wHealth, healthReq)
	if wHealth.Code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200 even with no DAO repository", wHealth.Code)
	}

	// An authenticated request to a protected route must fail closed
	// (503), not panic.
	authedReq := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	authedReq.Header.Set("Authorization", "Bearer "+tok)
	wAuthed := httptest.NewRecorder()
	router.ServeHTTP(wAuthed, authedReq)
	if wAuthed.Code != http.StatusServiceUnavailable {
		t.Errorf("authenticated request with no repository: status = %d, want 503", wAuthed.Code)
	}

	// An UNauthenticated request must still get 401, not 503 — auth
	// still runs before the repository is ever consulted.
	unauthedReq := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	wUnauthed := httptest.NewRecorder()
	router.ServeHTTP(wUnauthed, unauthedReq)
	if wUnauthed.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated request with no repository: status = %d, want 401 (auth must still run first)", wUnauthed.Code)
	}
}
