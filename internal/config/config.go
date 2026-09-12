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
	AppMode  string // "verifier" (default) | "issuer"
	HTTPAddr string

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
func Load() (Config, error) {
	dsn, err := resolveDSN()
	if err != nil {
		return Config{}, err
	}

	appMode := os.Getenv("APP_MODE")
	if appMode == "" {
		appMode = "verifier"
	}

	return Config{
		AppMode:  appMode,
		HTTPAddr: os.Getenv("HTTP_ADDR"),

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
