package app

import (
	"net/http"
	"time"

	"loginid-takehome/internal/api"
	"loginid-takehome/internal/config"
	"loginid-takehome/internal/dao"
)

// jwksCacheTTL matches the token TTL per handoff-03-auth.md v4's own
// guidance ("Verifier caches the fetched key set in memory with a short
// TTL (matching token TTL is reasonable)") — using the upper bound of the
// 5-15 minute TTL range so a cache refresh never fires more often than a
// token itself could have already expired and been re-issued.
const jwksCacheTTL = 15 * time.Minute

// NewVerifierRouter builds api-service's real, authenticated router —
// LT-39's handlers (search/retrieve) wired behind LT-40's JWT
// verification middleware, plus /healthz. This is the ModeVerifier
// Deployment's router; the issuer Deployment (ModeIssuer) still uses the
// plain NewRouter above, since it has no need for any of this — it never
// verifies inbound tokens, only issues them.
//
// repo may be nil (e.g. DB_DRIVER unset in a minimal smoke-test
// environment); the search/retrieve routes still register in that case,
// they'll simply fail at the DAO call with whatever error a nil
// Repository produces — this function's job is wiring, not validating
// that a database is reachable, which is /healthz's own job (LT-52).
func NewVerifierRouter(cfg config.Config, repo dao.Repository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler("api-service"))

	deps := api.Deps{
		Repo:        repo,
		RateLimiter: api.NewInProcessRateLimiter(),
		TouchCounter: api.NewInProcessTouchCounter(
			api.TouchCapPerWindow, api.TouchCapWindow),
		AuditLog: api.StdoutAuditLogger{},
		Authz: &api.StopgapAuthorizer{
			ReadOwnAllow: stopgapReadOwnAllow(cfg),
		},
	}

	keys := api.NewJWKSCache(cfg.AuthJWKSURL, jwksCacheTTL, nil)
	authMiddleware := api.NewJWTMiddleware(keys, cfg.AuthJWTIssuer, cfg.AuthJWTAudience)

	// api.NewDeadlineMiddleware outermost: the deadline budget must cover
	// JWT verification (including a JWKS cache-miss fetch) as well as
	// the handler's own DAO call, per decisions/context-propagation.md's
	// ruling — a single deadline, not the middleware's and the handler's
	// own disagreeing.
	protect := func(h http.Handler) http.Handler {
		return api.NewDeadlineMiddleware(authMiddleware(h))
	}

	mux.Handle("POST /profiles/search", protect(api.NewSearchHandler(deps)))
	mux.Handle("GET /profiles/{id}", protect(api.NewGetProfileHandler(deps)))

	return mux
}

// stopgapReadOwnAllow builds StopgapAuthorizer's hard-coded (sub ->
// allowed record id) map from cfg — empty (denies everything) unless
// both AuthzQASub and AuthzQAAllowedProfileID are set, which LT-51 does
// once the QA client credential and a matching seeded profile exist.
func stopgapReadOwnAllow(cfg config.Config) map[string]map[string]bool {
	if cfg.AuthzQASub == "" || cfg.AuthzQAAllowedProfileID == "" {
		return nil
	}
	return map[string]map[string]bool{
		cfg.AuthzQASub: {cfg.AuthzQAAllowedProfileID: true},
	}
}
