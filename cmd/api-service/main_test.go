package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"loginid-takehome/internal/app"
	"loginid-takehome/internal/config"
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
		garbage       string // one var to overwrite with a non-URL value after allSet, "" to skip
		wantErr       bool
		wantErrSubstr string
	}{
		{name: "all set: ok", wantErr: false},
		{name: "issuer missing", unset: "AUTH_JWT_ISSUER", wantErr: true, wantErrSubstr: "AUTH_JWT_ISSUER"},
		{name: "audience missing", unset: "AUTH_JWT_AUDIENCE", wantErr: true, wantErrSubstr: "AUTH_JWT_AUDIENCE"},
		{name: "jwks url missing", unset: "AUTH_JWKS_URL", wantErr: true, wantErrSubstr: "AUTH_JWKS_URL"},
		// Ingrid Solano's PR #53 review: a non-empty value that isn't a
		// valid URL must fail too, not just an empty one — Marcus
		// Ilori's (02) ruling narrowed the fix to AUTH_JWKS_URL and
		// AUTH_JWT_ISSUER specifically.
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

// TestValidateAuthConfig_SweepMode confirms LT-44's sweep mode requires
// none of the AUTH_* variables the other two modes need — it never
// verifies or issues a JWT — and that this is deliberate (recognized:
// true, empty required list), not the unrecognized-mode fallback.
func TestValidateAuthConfig_SweepMode(t *testing.T) {
	t.Setenv("AUTH_JWT_ISSUER", "")
	t.Setenv("AUTH_JWT_AUDIENCE", "")
	t.Setenv("AUTH_JWKS_URL", "")

	if err := validateAuthConfig(app.ModeSweep); err != nil {
		t.Errorf("validateAuthConfig(ModeSweep) = %v, want nil even with every AUTH_* var unset", err)
	}
}

// TestNewOnboardingService confirms LT-33's best-effort wiring: fully
// configured succeeds, and each individually-missing piece of config
// (the two connector-related URLs, or the client credential) is
// reported as an error rather than a Fatal-worthy panic — this
// capability must never crash-loop api-service over its own
// misconfiguration, per newOnboardingService's own doc comment.
func TestNewOnboardingService(t *testing.T) {
	full := func() config.Config {
		return config.Config{
			IssuerTokenURL:        "http://issuer.example/auth/token",
			IDPConnectorBaseURL:   "http://connector.example",
			ConnectorClientID:     "client-id",
			ConnectorClientSecret: "client-secret",
		}
	}

	if _, err := newOnboardingService(full()); err != nil {
		t.Errorf("newOnboardingService(fully configured) = %v, want nil", err)
	}

	cases := []struct {
		name   string
		mutate func(*config.Config)
	}{
		{name: "issuer token url missing", mutate: func(c *config.Config) { c.IssuerTokenURL = "" }},
		{name: "connector base url missing", mutate: func(c *config.Config) { c.IDPConnectorBaseURL = "" }},
		{name: "connector client id missing", mutate: func(c *config.Config) { c.ConnectorClientID = "" }},
		{name: "connector client secret missing", mutate: func(c *config.Config) { c.ConnectorClientSecret = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := full()
			c.mutate(&cfg)
			if _, err := newOnboardingService(cfg); err == nil {
				t.Errorf("newOnboardingService(%s) = nil error, want an error", c.name)
			}
		})
	}
}

// TestClassifySweepDBConnectError is Naomi Voss's (PO) live-run finding
// made concrete: a sweep pod hung silently against an unreachable
// database, and the fix's whole point is naming the failure class in
// one structured log line rather than leaving an operator to parse a
// raw driver error string.
func TestClassifySweepDBConnectError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: "connect_timeout"},
		{name: "wrapped deadline exceeded", err: fmt.Errorf("dialing: %w", context.DeadlineExceeded), want: "connect_timeout"},
		{name: "connection refused", err: errors.New("dial tcp 10.0.0.5:5432: connect: connection refused"), want: "connection_refused"},
		{name: "dns failure", err: errors.New("dial tcp: lookup postgres.example: no such host"), want: "dns_error"},
		{name: "unrecognized error text", err: errors.New("something else entirely"), want: "unknown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifySweepDBConnectError(c.err); got != c.want {
				t.Errorf("classifySweepDBConnectError(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}
}
