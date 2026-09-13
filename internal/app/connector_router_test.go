package app

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"loginid-takehome/internal/config"
	"loginid-takehome/internal/connector"
)

const testConnectorAudience = "idp-connector-service"

func signConnectorTestToken(t *testing.T, key *rsa.PrivateKey, kid, aud, scope string) string {
	t.Helper()
	c := jwt.MapClaims{
		"iss":   testIssuer,
		"aud":   aud,
		"sub":   "caller-under-test",
		"scope": scope,
		"exp":   time.Now().Add(10 * time.Minute).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("signing connector test token: %v", err)
	}
	return signed
}

func newTestStubVendor(t *testing.T) *connector.StubVendorClient {
	t.Helper()
	stub := connector.NewStubVendorClient()
	stub.Seed("demo-user", "demo-password", connector.Identity{Name: "Jane Demo", Country: "US"})
	return stub
}

func TestNewConnectorRouter_HealthzUnaffectedByAuth(t *testing.T) {
	cfg := config.Config{AuthJWTIssuer: testIssuer, ConnectorJWTAudience: testConnectorAudience, AuthJWKSURL: "http://unused.invalid"}
	router := NewConnectorRouter(cfg, newTestStubVendor(t))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — /healthz must stay unauthenticated", w.Code)
	}
}

func TestNewConnectorRouter_NoToken_401(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, ConnectorJWTAudience: testConnectorAudience, AuthJWKSURL: jwks.URL}
	router := NewConnectorRouter(cfg, newTestStubVendor(t))

	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"username":"demo-user","password":"demo-password"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 with no bearer token", w.Code)
	}
}

// TestNewConnectorRouter_WrongAudience_Rejected is LT-41's own named
// enforcement site (connector-security.md §5 / PO review script item
// 3): a token minted for api-service's audience (loginid-api-service)
// must not work against the connector, which requires its own distinct
// audience (idp-connector-service).
func TestNewConnectorRouter_WrongAudience_Rejected(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, ConnectorJWTAudience: testConnectorAudience, AuthJWKSURL: jwks.URL}
	router := NewConnectorRouter(cfg, newTestStubVendor(t))

	wrongAudienceToken := signConnectorTestToken(t, key, "key-1", "loginid-api-service", "connector:identity-lookup")
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"username":"demo-user","password":"demo-password"}`))
	req.Header.Set("Authorization", "Bearer "+wrongAudienceToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 — a loginid-api-service-audience token must be rejected by the connector's own aud check", w.Code)
	}
}

func TestNewConnectorRouter_CorrectAudienceAndScope_ReachesAuthHandler(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, ConnectorJWTAudience: testConnectorAudience, AuthJWKSURL: jwks.URL}
	router := NewConnectorRouter(cfg, newTestStubVendor(t))

	tok := signConnectorTestToken(t, key, "key-1", testConnectorAudience, "connector:identity-lookup")
	req := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"username":"demo-user","password":"demo-password"}`))
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

func TestNewConnectorRouter_IdentityEndToEnd(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	jwks := startFakeJWKS(t, "key-1", &key.PublicKey)
	defer jwks.Close()

	cfg := config.Config{AuthJWTIssuer: testIssuer, ConnectorJWTAudience: testConnectorAudience, AuthJWKSURL: jwks.URL}
	vendor := newTestStubVendor(t)
	router := NewConnectorRouter(cfg, vendor)
	tok := signConnectorTestToken(t, key, "key-1", testConnectorAudience, "connector:identity-lookup")

	// First mint a vendor token via /auth.
	authReq := httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"username":"demo-user","password":"demo-password"}`))
	authReq.Header.Set("Authorization", "Bearer "+tok)
	authW := httptest.NewRecorder()
	router.ServeHTTP(authW, authReq)
	if authW.Code != http.StatusOK {
		t.Fatalf("/auth status = %d, want 200, body=%s", authW.Code, authW.Body.String())
	}

	// Then use it against /identity.
	identityReq := httptest.NewRequest(http.MethodPost, "/identity", strings.NewReader(`{"phone":"+15555550100","name":"Jane"}`))
	identityReq.Header.Set("Authorization", "Bearer "+tok)
	identityReq.Header.Set("X-Vendor-Access-Token", extractAccessToken(t, authW.Body.String()))
	identityW := httptest.NewRecorder()
	router.ServeHTTP(identityW, identityReq)
	if identityW.Code != http.StatusOK {
		t.Fatalf("/identity status = %d, want 200, body=%s", identityW.Code, identityW.Body.String())
	}
	if !strings.Contains(identityW.Body.String(), "Jane Demo") {
		t.Errorf("/identity response missing expected identity: %s", identityW.Body.String())
	}
}

func extractAccessToken(t *testing.T, jsonBody string) string {
	t.Helper()
	// Minimal inline extraction — avoids importing internal/connector's
	// unexported response type from this package.
	const marker = `"access_token":"`
	i := strings.Index(jsonBody, marker)
	if i == -1 {
		t.Fatalf("no access_token field in %s", jsonBody)
	}
	rest := jsonBody[i+len(marker):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		t.Fatalf("malformed access_token field in %s", jsonBody)
	}
	return rest[:end]
}
