package app

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"loginid-takehome/internal/api"
	"loginid-takehome/internal/config"
)

// grantLimiterBaseBackoff/maxBackoff/alertThreshold are this story's own
// engineering defaults for api.GrantLimiter (handoff-03-auth.md v3
// requires the mechanism — failed-grant rate limiting/backoff per
// client_id and per source IP, plus alerting on a sustained run against
// one client_id — but names no specific numbers, unlike LT-40's
// touch-cap and JWKS-staleness constants, which 02 ruled explicitly).
// Doubling from 1s, capped at 5m, mirrors this codebase's other
// exponential-backoff component (InProcessRateLimiter's own doc
// comment); alerting after 5 consecutive failures against one client_id
// is a deliberately low bar — a real client mistyping a secret twice is
// not the failure mode this exists to catch, five in a row without an
// intervening success is. Flagged for 02's review rather than treated
// as a closed decision.
const (
	grantLimiterBaseBackoff    = 1 * time.Second
	grantLimiterMaxBackoff     = 5 * time.Minute
	grantLimiterAlertThreshold = 5
)

// NewIssuerRouter builds api-service's issuer-mode router — LT-51's real
// token-issuance handler and JWKS-serving endpoint, replacing the plain
// NewRouter's placeholder /auth/token (LT-32) on this Deployment only.
// The verifier Deployment never calls this — it has no signing key, no
// client store, and no legitimate use for either (LT-32's
// physical-absence criterion).
//
// key must be non-nil (cmd/api-service/main.go's config.Load already
// fails startup in issuer mode if JWT_SIGNING_KEY_FILE is missing or
// unreadable — see resolveSigningKeyPath's own doc comment — so by the
// time this function runs, a missing key is a startup bug, not a
// runtime condition this router needs to degrade gracefully for).
//
// issuerDB is the *sql.DB dialed against config.Config.IssuerDBDSN — may
// be nil if IssuerDBDSN was never configured, in which case /auth/token
// fails every grant closed with server_error rather than panicking on a
// nil ClientStore, the same fail-closed shape NewVerifierRouter uses for
// a nil dao.Repository.
func NewIssuerRouter(cfg config.Config, key *api.SigningKey, issuerDB *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler("api-service"))
	mux.HandleFunc("/.well-known/jwks.json", api.NewJWKSHandler(key))

	limiter := api.NewInProcessGrantLimiter(
		grantLimiterBaseBackoff, grantLimiterMaxBackoff, grantLimiterAlertThreshold,
		func(clientID string, consecutiveFailures int) {
			log.Printf("api-service: ALERT sustained failed grants against client_id=%s (%d consecutive)", clientID, consecutiveFailures)
		},
	)

	// store MUST stay a genuinely nil interface value when issuerDB is
	// nil, not a typed nil *PostgresClientStore boxed into a non-nil
	// ClientStore — tokenhandler.go's `store == nil` fail-closed check
	// only works because of exactly this construction (Oren Castellan,
	// PR #34 review). Do not "simplify" this to an unconditional
	// api.NewPostgresClientStore(issuerDB) call — that would silently
	// defeat the nil check with a non-nil interface wrapping a nil *sql.DB.
	var store api.ClientStore
	if issuerDB != nil {
		store = api.NewPostgresClientStore(issuerDB)
	}
	mux.Handle("POST /auth/token", api.NewTokenHandler(store, limiter, key, cfg.AuthJWTIssuer))

	return mux
}
