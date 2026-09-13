// Package config loads the env-var configuration surface published in root
// PLANNING.md's Service boundaries section. Both cmd/api-service and
// cmd/idp-connector load through this package so the surface is defined once.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds every environment variable named in PLANNING.md's Service
// boundaries section. Fields unused by S1's stub handlers are still loaded
// here so later stories don't have to touch this package's shape again.
type Config struct {
	AppMode  string // "verifier" (default) | "issuer" — see LT-32
	HTTPAddr string

	// JWTSigningKeyPath is the file path to the JWT signing private key
	// (handoff-04-secrets.md row #2 — "runtime-injected as a file, never
	// an env var"). Only meaningful when AppMode == "issuer"; empty in
	// verifier mode, since the verifying Deployment's pod spec never
	// mounts this Secret at all (LT-32's physical-absence criterion) and
	// therefore has no path set to point at.
	JWTSigningKeyPath string

	DBDriver string // postgres | cockroachdb | sqlite
	DBDSN    string // resolved value: DB_DSN_FILE content takes precedence over DB_DSN

	AuthJWTIssuer   string
	AuthJWTAudience string

	ConnectorJWTAudience  string
	ConnectorClientID     string
	ConnectorClientSecret string

	IDPABCBaseURL      string
	IDPABCClientID     string
	IDPABCClientSecret string
}

// Load reads the process environment into a Config. DB_DSN_FILE, when set,
// is read and its trimmed content used as DBDSN in preference to DB_DSN —
// DB_DSN is a transitional fallback, not a permanent equal option, per
// handoff-04-secrets.md's S7 revision.
//
// APP_MODE is validated, not merely defaulted: an unrecognized value is a
// startup error rather than a silent fall-through to "verifier", because a
// typo'd issuer Deployment silently running as a verifier (or vice versa)
// is exactly the class of misconfiguration LT-32 exists to make loud
// instead of invisible.
//
// In issuer mode, JWT_SIGNING_KEY_FILE must be set and the file it names
// must actually be readable, checked here at startup rather than deferred
// to whatever first tries to use it. This is LT-32's defense-in-depth
// criterion: the verifying Deployment's pod spec never mounts this Secret
// at all (enforced on 04's side, by physical absence, not by this check),
// so if APP_MODE were ever misconfigured to "issuer" on that Deployment,
// this still fails startup rather than serving traffic in a half-issuer
// state with no actual path to the private key.
func Load() (Config, error) {
	dsn, err := resolveDSN()
	if err != nil {
		return Config{}, err
	}

	appMode := os.Getenv("APP_MODE")
	if appMode == "" {
		appMode = "verifier"
	}
	if appMode != "verifier" && appMode != "issuer" {
		return Config{}, fmt.Errorf("config: APP_MODE %q is not one of \"verifier\", \"issuer\"", appMode)
	}

	signingKeyPath, err := resolveSigningKeyPath(appMode)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppMode:           appMode,
		JWTSigningKeyPath: signingKeyPath,
		HTTPAddr:          os.Getenv("HTTP_ADDR"),

		DBDriver: os.Getenv("DB_DRIVER"),
		DBDSN:    dsn,

		AuthJWTIssuer:   os.Getenv("AUTH_JWT_ISSUER"),
		AuthJWTAudience: os.Getenv("AUTH_JWT_AUDIENCE"),

		ConnectorJWTAudience:  os.Getenv("CONNECTOR_JWT_AUDIENCE"),
		ConnectorClientID:     os.Getenv("CONNECTOR_CLIENT_ID"),
		ConnectorClientSecret: os.Getenv("CONNECTOR_CLIENT_SECRET"),

		IDPABCBaseURL:      os.Getenv("IDP_ABC_BASE_URL"),
		IDPABCClientID:     os.Getenv("IDP_ABC_CLIENT_ID"),
		IDPABCClientSecret: os.Getenv("IDP_ABC_CLIENT_SECRET"),
	}, nil
}

// resolveSigningKeyPath enforces LT-32's issuer-mode requirement: in
// "issuer" mode, JWT_SIGNING_KEY_FILE must name a file this process can
// actually open, checked now rather than left for whatever code first
// tries to sign a token — a startup failure here is the only thing that
// distinguishes "issuer mode with no key" from "issuer mode with a key",
// and the whole point of the physical-absence criterion is that those two
// must never be confused for one another. In "verifier" mode the variable
// is not read at all: the verifying binary has no legitimate use for a
// signing-key path, set or not.
func resolveSigningKeyPath(appMode string) (string, error) {
	if appMode != "issuer" {
		return "", nil
	}
	path := os.Getenv("JWT_SIGNING_KEY_FILE")
	if path == "" {
		return "", fmt.Errorf("config: APP_MODE=issuer requires JWT_SIGNING_KEY_FILE to be set")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("config: APP_MODE=issuer set but JWT_SIGNING_KEY_FILE %q is not readable: %w", path, err)
	}
	_ = f.Close()
	return path, nil
}

func resolveDSN() (string, error) {
	if path := os.Getenv("DB_DSN_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("config: reading DB_DSN_FILE %q: %w", path, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return os.Getenv("DB_DSN"), nil
}
