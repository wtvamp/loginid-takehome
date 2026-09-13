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
