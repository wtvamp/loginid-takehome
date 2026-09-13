package main

import (
	"strings"
	"testing"
)

// TestValidateAuthConfig is the live-review finding's own enforcement
// site: AUTH_JWKS_URL and AUTH_JWT_ISSUER were both silently absent from
// idp-connector's Deployment manifest, so every request 401'd in a way
// indistinguishable from an ordinary rejected token — this must fail
// startup loudly instead, naming exactly which variable is missing.
func TestValidateAuthConfig(t *testing.T) {
	allSet := func(t *testing.T) {
		t.Helper()
		t.Setenv("AUTH_JWT_ISSUER", "https://issuer.example")
		t.Setenv("AUTH_JWKS_URL", "http://api-service-issuer.loginid-takehome.svc.cluster.local:443/.well-known/jwks.json")
		t.Setenv("CONNECTOR_JWT_AUDIENCE", "idp-connector-service")
	}

	cases := []struct {
		name          string
		unset         string
		garbage       string
		wantErr       bool
		wantErrSubstr string
	}{
		{name: "all set: ok", wantErr: false},
		{name: "issuer missing", unset: "AUTH_JWT_ISSUER", wantErr: true, wantErrSubstr: "AUTH_JWT_ISSUER"},
		{name: "jwks url missing", unset: "AUTH_JWKS_URL", wantErr: true, wantErrSubstr: "AUTH_JWKS_URL"},
		{name: "connector audience missing", unset: "CONNECTOR_JWT_AUDIENCE", wantErr: true, wantErrSubstr: "CONNECTOR_JWT_AUDIENCE"},
		// Ingrid Solano's PR #53 review + Marcus Ilori's (02) ruling:
		// a non-empty but non-URL value must fail too.
		{name: "jwks url garbage value", garbage: "AUTH_JWKS_URL", wantErr: true, wantErrSubstr: "AUTH_JWKS_URL"},
		{name: "issuer garbage value", garbage: "AUTH_JWT_ISSUER", wantErr: true, wantErrSubstr: "AUTH_JWT_ISSUER"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			allSet(t)
			if c.unset != "" {
				t.Setenv(c.unset, "")
			}
			if c.garbage != "" {
				t.Setenv(c.garbage, "not-a-url")
			}
			err := validateAuthConfig()
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
