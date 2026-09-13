package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestMintThenVerify_RealTokenThroughRealJWTMiddleware is LT-51's own
// integration proof: a token minted by NewTokenHandler (this story) is
// accepted by NewJWTMiddleware (LT-40, PR #30) fetching the signing
// key's public half over a real HTTP JWKS endpoint (NewJWKSHandler, this
// story) — not two components tested in isolation and assumed to agree,
// the actual mint-then-verify path end to end, using the real JWKS
// cache and its real HTTP fetch, not a fake KeySource.
func TestMintThenVerify_RealTokenThroughRealJWTMiddleware(t *testing.T) {
	key := testSigningKey(t)

	jwksSrv := httptest.NewServer(NewJWKSHandler(key))
	defer jwksSrv.Close()

	hash, err := hashSecret("s3cr3t")
	if err != nil {
		t.Fatalf("hashing secret: %v", err)
	}
	store := &fakeClientStore{records: map[string]ClientRecord{
		"integration-client": {
			ClientID:         "integration-client",
			Name:             "Integration Test Client",
			ClientSecretHash: hash,
			GrantedScope:     Scope("profile:search"),
			Audience:         "loginid-api-service",
			Active:           true,
		},
	}}
	limiter := &fakeGrantLimiter{allow: true}
	tokenHandler := NewTokenHandler(store, limiter, key, "https://issuer.test")

	tokenReq := httptest.NewRequest(http.MethodPost, "/auth/token", nil)
	tokenReq.Method = http.MethodPost
	tokenReq.Form = map[string][]string{"grant_type": {"client_credentials"}}
	tokenReq.PostForm = tokenReq.Form
	tokenReq.SetBasicAuth("integration-client", "s3cr3t")
	tokenW := httptest.NewRecorder()
	tokenHandler(tokenW, tokenReq)
	if tokenW.Code != http.StatusOK {
		t.Fatalf("minting token: status = %d, body=%s", tokenW.Code, tokenW.Body.String())
	}
	var tokenBody tokenResponse
	if err := json.Unmarshal(tokenW.Body.Bytes(), &tokenBody); err != nil {
		t.Fatalf("decoding token response: %v", err)
	}

	keys := NewJWKSCache(jwksSrv.URL, 15*time.Minute, nil)
	verify := NewJWTMiddleware(keys, "https://issuer.test", "loginid-api-service", StdoutAuditLogger{})

	var gotAuthContext AuthContext
	var gotOK bool
	protected := verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthContext, gotOK = AuthContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	verifyReq := httptest.NewRequest(http.MethodPost, "/profiles/search", nil)
	verifyReq.Header.Set("Authorization", "Bearer "+tokenBody.AccessToken)
	verifyW := httptest.NewRecorder()
	protected.ServeHTTP(verifyW, verifyReq)

	if verifyW.Code != http.StatusOK {
		t.Fatalf("verifying minted token: status = %d, want 200 (mint-then-verify must round-trip through the real JWKS-over-HTTP path)", verifyW.Code)
	}
	if !gotOK {
		t.Fatalf("AuthContext was never populated in the protected handler")
	}
	if gotAuthContext.Sub != "integration-client" {
		t.Errorf("AuthContext.Sub = %q, want integration-client", gotAuthContext.Sub)
	}
	if gotAuthContext.Scope != Scope("profile:search") {
		t.Errorf("AuthContext.Scope = %q, want profile:search", gotAuthContext.Scope)
	}
}

