package main

import (
	"strings"
	"testing"

	"loginid-takehome/internal/app"
)

// TestValidateAuthConfig_VerifierMode is the live-review finding's own
// enforcement site: AUTH_JWT_ISSUER, AUTH_JWT_AUDIENCE, and AUTH_JWKS_URL
// all missing from the deployed manifests (in two separate incidents)
// let the verifier come up cleanly and fail in a way that looked like an
// ordinary token rejection — this must fail startup loudly instead,
// naming exactly which variable is missing. validateAuthConfig reads the
// process environment directly (via config.RequiredAuthEnvVars), so
// these tests set/unset real env vars rather than constructing a
// config.Config.
func TestValidateAuthConfig_VerifierMode(t *testing.T) {
	allSet := func(t *testing.T) {
		t.Helper()
		t.Setenv("AUTH_JWT_ISSUER", "https://issuer.example")
		t.Setenv("AUTH_JWT_AUDIENCE", "loginid-api-service")
		t.Setenv("AUTH_JWKS_URL", "http://api-service-issuer.loginid-takehome.svc.cluster.local:443/.well-known/jwks.json")
	}

	cases := []struct {
		name          string
		unset         string // one var to blank out after allSet, "" for the all-set case
		wantErr       bool
		wantErrSubstr string
	}{
		{name: "all set: ok", wantErr: false},
		{name: "issuer missing", unset: "AUTH_JWT_ISSUER", wantErr: true, wantErrSubstr: "AUTH_JWT_ISSUER"},
		{name: "audience missing", unset: "AUTH_JWT_AUDIENCE", wantErr: true, wantErrSubstr: "AUTH_JWT_AUDIENCE"},
		{name: "jwks url missing", unset: "AUTH_JWKS_URL", wantErr: true, wantErrSubstr: "AUTH_JWKS_URL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allSet(t)
			if c.unset != "" {
				t.Setenv(c.unset, "")
			}
			err := validateAuthConfig(app.ModeVerifier)
			if c.wantErr && err == nil {
				t.Fatalf("validateAuthConfig() = nil, want an error")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("validateAuthConfig() = %v, want nil", err)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.wantErrSubstr) {
				t.Errorf("error = %q, want it to name %q", err, c.wantErrSubstr)
			}
		})
	}
}

// TestValidateAuthConfig_IssuerMode confirms issuer mode only requires
// AUTH_JWT_ISSUER — AUTH_JWT_AUDIENCE and AUTH_JWKS_URL are meaningless
// here (the issuer mints each token's aud from the per-client
// oauth_client.audience column, not from global config, and never
// verifies inbound tokens so it has no JWKS to fetch), so an issuer
// Deployment with neither set at all must NOT be rejected.
func TestValidateAuthConfig_IssuerMode(t *testing.T) {
	cases := []struct {
		name      string
		setIssuer bool
		wantErr   bool
	}{
		{name: "issuer set, audience/jwks unset: ok", setIssuer: true, wantErr: false},
		{name: "issuer missing", setIssuer: false, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("AUTH_JWT_ISSUER", "")
			t.Setenv("AUTH_JWT_AUDIENCE", "")
			t.Setenv("AUTH_JWKS_URL", "")
			if c.setIssuer {
				t.Setenv("AUTH_JWT_ISSUER", "https://issuer.example")
			}
			err := validateAuthConfig(app.ModeIssuer)
			if c.wantErr && err == nil {
				t.Fatalf("validateAuthConfig() = nil, want an error")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("validateAuthConfig() = %v, want nil", err)
			}
			if c.wantErr && !strings.Contains(err.Error(), "AUTH_JWT_ISSUER") {
				t.Errorf("error = %q, want it to name AUTH_JWT_ISSUER", err)
			}
		})
	}
}
