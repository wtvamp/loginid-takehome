package main

import (
	"strings"
	"testing"

	"loginid-takehome/internal/app"
	"loginid-takehome/internal/config"
)

// TestValidateAuthConfig_VerifierMode is the live-review finding's own
// enforcement site: AUTH_JWT_ISSUER and AUTH_JWT_AUDIENCE both missing
// from the deployed manifests let the verifier silently compare every
// token's aud claim against "" — this must fail startup loudly instead,
// naming exactly which variable is missing.
func TestValidateAuthConfig_VerifierMode(t *testing.T) {
	cases := []struct {
		name          string
		cfg           config.Config
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:    "both set: ok",
			cfg:     config.Config{AuthJWTIssuer: "https://issuer.example", AuthJWTAudience: "loginid-api-service"},
			wantErr: false,
		},
		{
			name:          "issuer missing",
			cfg:           config.Config{AuthJWTIssuer: "", AuthJWTAudience: "loginid-api-service"},
			wantErr:       true,
			wantErrSubstr: "AUTH_JWT_ISSUER",
		},
		{
			name:          "audience missing",
			cfg:           config.Config{AuthJWTIssuer: "https://issuer.example", AuthJWTAudience: ""},
			wantErr:       true,
			wantErrSubstr: "AUTH_JWT_AUDIENCE",
		},
		{
			name:    "both missing",
			cfg:     config.Config{},
			wantErr: true,
			// Either name is acceptable — the point is a loud, named
			// failure, not a specific ordering between the two checks.
			wantErrSubstr: "AUTH_JWT_ISSUER",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAuthConfig(c.cfg, app.ModeVerifier)
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
// AUTH_JWT_ISSUER — AUTH_JWT_AUDIENCE is meaningless here (the issuer
// mints each token's aud from the per-client oauth_client.audience
// column, not from global config), so an issuer Deployment with no
// AUTH_JWT_AUDIENCE set at all must NOT be rejected.
func TestValidateAuthConfig_IssuerMode(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{
			name:    "issuer set, audience empty: ok",
			cfg:     config.Config{AuthJWTIssuer: "https://issuer.example", AuthJWTAudience: ""},
			wantErr: false,
		},
		{
			name:    "issuer missing",
			cfg:     config.Config{AuthJWTIssuer: ""},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAuthConfig(c.cfg, app.ModeIssuer)
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
