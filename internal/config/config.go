// Package config loads the env-var configuration surface published in root
// PLANNING.md's Service boundaries section. Both cmd/api-service and
// cmd/idp-connector load through this package so the surface is defined once.
package config

import (
	"fmt"
	"net/url"
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
	// AuthJWKSURL is the verifier's only path to public key material
	// (LT-40, handoff-03-auth.md v4): the issuer Deployment's in-cluster
	// ClusterIP Service JWKS endpoint, e.g.
	// "http://api-service-issuer.loginid-takehome.svc.cluster.local/.well-known/jwks.json".
	// Never a mounted key file — JWKS survives rotation without a config
	// change, since the verifier fetches by kid.
	AuthJWKSURL string

	// AuthzQASub and AuthzQAAllowedProfileID seed the temporary,
	// default-deny StopgapAuthorizer (internal/api) for LT-40/LT-51's
	// joint live-URL review only — see StopgapAuthorizer's own doc
	// comment for why a permissive default was explicitly ruled out.
	// Both empty (the default) means the stand-in denies every
	// profile:read:own request, the safe default until LT-51 seeds a
	// real QA client credential and this is set to match it.
	AuthzQASub              string
	AuthzQAAllowedProfileID string

	// IssuerDBDSN is the token issuer's own database connection string —
	// a separate PostgreSQL database (`issuer`, per Priya Nandakumar's
	// (05) ruling on LT-51: same instance as the main DAO's database, but
	// a distinct database so a cross-database join to user_profile is
	// structurally impossible), holding only oauth_client. Resolved the
	// same way DBDSN is (ISSUER_DB_DSN_FILE takes precedence over
	// ISSUER_DB_DSN), matching handoff-04-secrets.md's discipline. Only
	// meaningful in issuer mode — empty in verifier mode, which has no
	// legitimate use for it. There is no ISSUER_DB_DRIVER: the issuer
	// database is always PostgreSQL (no SQLite/CockroachDB peer was ever
	// specified for oauth_client), so internal/api's client store dials
	// it directly via the "pgx" driver rather than routing through
	// dao.New's multi-backend factory.
	IssuerDBDSN string

	// QA client credential seeding is out-of-band (a runbook 04 owns
	// against the real oauth_client table), never application code or a
	// migration — Priya's explicit ruling on this story. There is
	// deliberately no QAClientID/QAClientSecretFile/etc. config surface
	// here for auto-seeding a client at startup; AuthzQASub above is the
	// only piece of QA-review config this track owns, and it's set to
	// match whatever client_id the runbook seeds, not the other way
	// around.

	ConnectorJWTAudience string

	// ConnectorClientID/ConnectorClientSecret are api-service's own
	// client-credentials for obtaining a connector:identity-lookup
	// token from the issuer (LT-33, handoff-04-secrets.md row 10).
	// Resolved the same *_FILE-takes-precedence way DBDSN is
	// (CONNECTOR_CLIENT_ID_FILE/CONNECTOR_CLIENT_SECRET_FILE win over
	// the plain env vars when set) so this package stays indifferent to
	// whether the value is delivered as a mounted Kubernetes Secret or
	// (per Warren's approval, LT-46) a Vault Agent Injector-rendered
	// file — the delivery mechanism is 04's concern, not this one's.
	ConnectorClientID     string
	ConnectorClientSecret string

	// IssuerTokenURL is where api-service itself calls POST /auth/token
	// to obtain its own connector-scoped token (LT-33) — the issuer
	// Deployment's in-cluster ClusterIP Service, e.g.
	// "http://api-service-issuer.loginid-takehome.svc.cluster.local:443/auth/token".
	// Not an auth-enforcement env var in RequiredAuthEnvVars's sense
	// (there's no "must be set for tokens to verify" incident this
	// guards against); internal/onboarding.NewTokenSource validates its
	// presence directly, the same as the connector client credential
	// above.
	IssuerTokenURL string

	// IDPConnectorBaseURL is where api-service calls idp-connector's own
	// /auth and /identity (LT-33) — the connector Deployment's in-cluster
	// ClusterIP Service, e.g.
	// "http://idp-connector.loginid-takehome.svc.cluster.local:443".
	IDPConnectorBaseURL string

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
	dsn, err := resolveDSNFrom("DB_DSN_FILE", "DB_DSN")
	if err != nil {
		return Config{}, err
	}

	issuerDSN, err := resolveDSNFrom("ISSUER_DB_DSN_FILE", "ISSUER_DB_DSN")
	if err != nil {
		return Config{}, err
	}

	// CONNECTOR_CLIENT_ID/SECRET follow the same *_FILE-takes-precedence
	// pattern as DB_DSN/ISSUER_DB_DSN above (handoff-04-secrets.md row
	// 10) — Warren's approval to move to Vault Agent Injector delivery
	// (LT-46, Theo's story) means this code must stay indifferent to
	// whether the value arrives via a plain Kubernetes Secret or a Vault
	// Agent-rendered file; reusing resolveDSNFrom's existing
	// file-or-env-var resolution is what makes that true without this
	// package caring which mechanism actually wrote the file.
	connectorClientID, err := resolveDSNFrom("CONNECTOR_CLIENT_ID_FILE", "CONNECTOR_CLIENT_ID")
	if err != nil {
		return Config{}, err
	}
	connectorClientSecret, err := resolveDSNFrom("CONNECTOR_CLIENT_SECRET_FILE", "CONNECTOR_CLIENT_SECRET")
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

		IssuerDBDSN: issuerDSN,

		AuthJWTIssuer:   os.Getenv("AUTH_JWT_ISSUER"),
		AuthJWTAudience: os.Getenv("AUTH_JWT_AUDIENCE"),
		AuthJWKSURL:     os.Getenv("AUTH_JWKS_URL"),

		AuthzQASub:              os.Getenv("AUTHZ_QA_SUB"),
		AuthzQAAllowedProfileID: os.Getenv("AUTHZ_QA_ALLOWED_PROFILE_ID"),

		ConnectorJWTAudience:  os.Getenv("CONNECTOR_JWT_AUDIENCE"),
		ConnectorClientID:     connectorClientID,
		ConnectorClientSecret: connectorClientSecret,

		IssuerTokenURL:      os.Getenv("ISSUER_TOKEN_URL"),
		IDPConnectorBaseURL: os.Getenv("IDP_CONNECTOR_BASE_URL"),

		IDPABCBaseURL:      os.Getenv("IDP_ABC_BASE_URL"),
		IDPABCClientID:     os.Getenv("IDP_ABC_CLIENT_ID"),
		IDPABCClientSecret: os.Getenv("IDP_ABC_CLIENT_SECRET"),
	}, nil
}

// resolveSigningKeyPath enforces LT-32's issuer-mode requirement: in
// "issuer" mode, JWT_SIGNING_KEY_FILE must name a regular file this
// process can actually read at least one byte of, checked now rather than
// left for whatever code first tries to sign a token — a startup failure
// here is the only thing that distinguishes "issuer mode with no key"
// from "issuer mode with a key", and the whole point of the
// physical-absence criterion is that those two must never be confused for
// one another. In "verifier" mode the variable is not read at all: the
// verifying binary has no legitimate use for a signing-key path, set or
// not.
//
// This checks openable-and-non-empty at startup, not "will still be a
// valid key when S7 reads it" — a Secret rotated to empty or a volume
// re-provisioned later isn't something a one-time startup check can see
// (Nolan Reyes, PR #18 review). That's S7's problem to solve when it
// exists, not this story's: LT-32 only owns making a *missing* key loud
// at startup, not monitoring a *present* key's ongoing validity.
func resolveSigningKeyPath(appMode string) (string, error) {
	if appMode != "issuer" {
		return "", nil
	}
	path := os.Getenv("JWT_SIGNING_KEY_FILE")
	if path == "" {
		return "", fmt.Errorf("config: APP_MODE=issuer requires JWT_SIGNING_KEY_FILE to be set")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("config: APP_MODE=issuer set but JWT_SIGNING_KEY_FILE %q is not accessible: %w", path, err)
	}
	// os.Open succeeds on a directory as readily as on a regular file —
	// no error, nothing distinguishing the two. JWT_SIGNING_KEY_FILE
	// pointed at a mount directory instead of the file inside it (an easy
	// Kubernetes subPath-vs-mount-root mistake, and exactly the class of
	// misconfiguration this story exists to catch) would otherwise pass
	// this check and only fail later, whenever S7's signing code first
	// tries to read it — deferring the failure this check exists to make
	// immediate (Oren Castellan, PR #18 review).
	if fi.IsDir() {
		return "", fmt.Errorf("config: APP_MODE=issuer set but JWT_SIGNING_KEY_FILE %q is a directory, not a file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("config: APP_MODE=issuer set but JWT_SIGNING_KEY_FILE %q is not readable: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	// Read one byte rather than open-close: an empty file (a Secret
	// mounted but never populated) opens without error and would
	// otherwise pass silently.
	var buf [1]byte
	if _, err := f.Read(buf[:]); err != nil {
		return "", fmt.Errorf("config: APP_MODE=issuer set but JWT_SIGNING_KEY_FILE %q is empty or unreadable: %w", path, err)
	}
	return path, nil
}

// resolveDSNFrom implements the DB_DSN_FILE-takes-precedence-over-DB_DSN
// pattern generically, parameterized by env var name so it serves both
// the main DAO's DSN and IssuerDBDSN without duplicating the logic.
func resolveDSNFrom(fileEnv, plainEnv string) (string, error) {
	if path := os.Getenv(fileEnv); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("config: reading %s %q: %w", fileEnv, path, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return os.Getenv(plainEnv), nil
}

// RequiredAuthEnvVars is the single source of truth for which auth-related
// env vars a given (service, mode) combination MUST have set to function
// correctly — read by both this process's own startup validation
// (cmd/api-service/main.go's validateAuthConfig, cmd/idp-connector/main.go's
// equivalent) and, independently, by internal/config's own
// TestManifestsSetRequiredAuthEnvVars, which parses the actual deployed
// manifests (deploy/api-service.yaml, deploy/manifests.yaml) and asserts
// every Deployment sets everything its own (service, mode) requires.
//
// Exists because of two separate live-review incidents in one day, both
// the same shape: a real, correctly-signed token failed verification
// because a required auth env var was silently absent from a
// Deployment's manifest, and nothing caught it before a human noticed
// the symptom in production. cmd/api-service/main.go's own
// validateAuthConfig (added after the first incident) only checks the
// binary it lives in — it has no way to know cmd/idp-connector's
// Deployment was missing AUTH_JWKS_URL/AUTH_JWT_ISSUER entirely (the
// second incident), because idp-connector never runs that check at all
// until this same list is wired into its own main() too. And neither
// check, however complete, can catch a manifest that's simply missing
// the variable outright before a real request exercises the gap — only
// a test that reads the manifest text itself can do that ahead of time,
// which is what this declaration exists to make possible.
//
// service is "api-service" or "idp-connector" — the binary, not the
// Deployment name (api-service and api-service-issuer are the SAME
// binary in different modes). mode is "verifier"/"issuer" for
// api-service (matching AppMode's own two values), or "" for
// idp-connector, which has no mode concept.
// The second return value, recognized, distinguishes "this (service, mode)
// pair is known and legitimately requires nothing extra" from "this pair
// doesn't match anything this function knows about" — a typo'd mode string
// (e.g. a manifest's APP_MODE misspelled, or a startup caller passing a
// value that isn't one of app.Mode's own constants) must not silently
// require zero env vars, since that would make both enforcement sites
// (startup validation and the manifest test) blind to the exact class of
// gap this declaration exists to catch, just moved one level up into its
// own lookup (Oren Castellan, PR #53 review).
func RequiredAuthEnvVars(service, mode string) (required []string, recognized bool) {
	switch {
	case service == "api-service" && mode == "verifier":
		return []string{"AUTH_JWT_ISSUER", "AUTH_JWT_AUDIENCE", "AUTH_JWKS_URL"}, true
	case service == "api-service" && mode == "issuer":
		// AUTH_JWT_AUDIENCE deliberately absent here — the issuer mints
		// each token's aud from the per-client oauth_client.audience
		// column (tokenhandler.go), never from its own config, so
		// requiring it would reject a valid issuer configuration for a
		// variable it has no use for (validateAuthConfig's own doc
		// comment states this same reasoning).
		return []string{"AUTH_JWT_ISSUER"}, true
	case service == "idp-connector":
		return []string{"AUTH_JWT_ISSUER", "AUTH_JWKS_URL", "CONNECTOR_JWT_AUDIENCE"}, true
	default:
		return nil, false
	}
}

// urlShapedEnvVars is the narrow set RequiredAuthEnvVars callers also
// check for URL shape, not just presence — Ingrid Solano's review of PR
// #53 (an env var can be non-empty garbage and still pass a bare
// presence check) and Marcus Ilori's (02) ruling narrowing the fix to
// exactly these two: AUTH_JWKS_URL and AUTH_JWT_ISSUER are the two
// values this project's own live incidents actually broke on (empty
// string, "unsupported protocol scheme \"\""), and both are meant to be
// absolute URLs the verifier/connector dial or compare against directly.
// Deliberately not extended to every other env var (client IDs, audience
// strings, etc. have no comparable "must be a URL" shape to check) —
// Marcus's ruling: "presence-plus-URL-shape is the control, don't extend
// it further."
var urlShapedEnvVars = map[string]bool{
	"AUTH_JWKS_URL":   true,
	"AUTH_JWT_ISSUER": true,
}

// ValidateEnvVarValue rejects a structurally implausible value for env
// vars in urlShapedEnvVars — value already known non-empty (presence is
// each RequiredAuthEnvVars caller's own, separate check); this only
// catches the "non-empty but garbage" case a bare presence check can't.
// A no-op for every other env var name.
func ValidateEnvVarValue(name, value string) error {
	if !urlShapedEnvVars[name] {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s=%q does not look like a valid absolute URL (a scheme and host are required)", name, value)
	}
	return nil
}

// OptionalEnvVars is every env var config.Load reads that is NOT gated
// by RequiredAuthEnvVars, paired with the one-line reason it's exempt —
// Marcus Ilori's (02) ruling on Ingrid Solano's second PR #53 objection:
// a hand-maintained required-list with nothing checking it against the
// code's own env reads is a convention, not a control, since a future
// AUTH_*-prefixed field added straight to Load() (or read directly by
// some other code, bypassing this package entirely) would silently
// never be enforced by either the startup checks or the manifest test.
// internal/config's own TestConfigGoEnvVarsAreClassified scans
// config.go's source for every env-var-shaped string literal and
// requires each one to appear either in some RequiredAuthEnvVars
// (service, mode) result or here — an unclassified new read fails that
// test, not just an unenforced one.
var OptionalEnvVars = map[string]string{
	"APP_MODE":                    "validated by config.Load's own dedicated check (must be \"verifier\" or \"issuer\"), not by RequiredAuthEnvVars",
	"JWT_SIGNING_KEY_FILE":        "validated by resolveSigningKeyPath's own issuer-mode-only check, not by RequiredAuthEnvVars",
	"HTTP_ADDR":                   "listen address; no auth-enforcement concern",
	"DB_DRIVER":                   "database driver selector; no auth-enforcement concern",
	"DB_DSN":                      "database DSN fallback; no auth-enforcement concern",
	"DB_DSN_FILE":                 "database DSN source; no auth-enforcement concern",
	"ISSUER_DB_DSN":               "issuer database DSN fallback; no auth-enforcement concern",
	"ISSUER_DB_DSN_FILE":          "issuer database DSN source; no auth-enforcement concern",
	"AUTHZ_QA_SUB":                "QA stopgap-authorizer seed, intentionally optional — empty means deny-by-default, per StopgapAuthorizer's own doc comment",
	"AUTHZ_QA_ALLOWED_PROFILE_ID": "QA stopgap-authorizer seed, intentionally optional — empty means deny-by-default, per StopgapAuthorizer's own doc comment",
	// CONNECTOR_CLIENT_ID/SECRET (LT-33) are NOT folded into
	// RequiredAuthEnvVars: that function's model is "is this literal
	// env var name non-empty," which doesn't fit the *_FILE-takes-
	// precedence resolution these two go through (a Vault Agent
	// Injector delivery, per LT-46, may set only the _FILE variant and
	// leave the plain var genuinely empty on purpose). Presence is
	// instead validated where the resolved value is actually consumed:
	// internal/onboarding.NewTokenSource returns an error if
	// cfg.ConnectorClientID/ConnectorClientSecret are empty after
	// Load's own file-or-env resolution already ran.
	"CONNECTOR_CLIENT_ID":          "resolved via *_FILE precedence into Config.ConnectorClientID; presence validated by internal/onboarding.NewTokenSource, not RequiredAuthEnvVars",
	"CONNECTOR_CLIENT_ID_FILE":     "optional alternative delivery path for CONNECTOR_CLIENT_ID — see that entry",
	"CONNECTOR_CLIENT_SECRET":      "resolved via *_FILE precedence into Config.ConnectorClientSecret; presence validated by internal/onboarding.NewTokenSource, not RequiredAuthEnvVars",
	"CONNECTOR_CLIENT_SECRET_FILE": "optional alternative delivery path for CONNECTOR_CLIENT_SECRET — see that entry",
	"IDP_ABC_BASE_URL":             "optional real-vendor base URL; unset selects the stub vendor client by design (LT-41 non-goal: no real vendor exists for this take-home)",
	"IDP_ABC_CLIENT_ID":            "vendor's own credential surface, not this project's own auth surface",
	"IDP_ABC_CLIENT_SECRET":        "vendor's own credential surface, not this project's own auth surface",
	"ISSUER_TOKEN_URL":             "where api-service calls the issuer for its own connector-scoped token; presence validated by internal/onboarding.NewTokenSource, not RequiredAuthEnvVars",
	"IDP_CONNECTOR_BASE_URL":       "where api-service calls idp-connector's /auth and /identity; presence validated by internal/onboarding.NewTokenSource, not RequiredAuthEnvVars",
}
