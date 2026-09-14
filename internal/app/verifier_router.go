package app

import (
	"database/sql"
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
// repo may be nil — cmd/api-service/main.go passes nil rather than
// crash-looping the whole process when the DAO repository couldn't be
// constructed at startup (no DB_DRIVER/DB_DSN yet, e.g. before the
// database exists in this environment). /healthz must stay reachable
// regardless (LT-34's live-URL story); the two protected routes fail
// closed with 503 instead of panicking on a nil Repository or silently
// pretending to work.
//
// pingDB (LT-52 criterion 5) is a separate, lightweight *sql.DB used
// only for /healthz's bounded connectivity probe — deliberately not the
// dao.Repository above, since 05's Repository contract exposes no Ping
// and this story has no reason to ask for one just for a health check.
// pingDB may be nil (same startup-failure path as repo above); either
// way /healthz keeps reporting "status":"ok" and "db":"down" rather
// than going unreachable — a DB outage is this router's problem to fail
// the two DB-backed routes closed on, not a reason to take the whole
// process's liveness probe down with it.
func NewVerifierRouter(cfg config.Config, repo dao.Repository, pingDB *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandlerWithDB("api-service", pingDB))

	auditLog := api.StdoutAuditLogger{}
	keys := api.NewJWKSCache(cfg.AuthJWKSURL, jwksCacheTTL, nil)
	authMiddleware := api.NewJWTMiddleware(keys, cfg.AuthJWTIssuer, cfg.AuthJWTAudience, auditLog)

	// api.NewDeadlineMiddleware outermost: the deadline budget must cover
	// JWT verification (including a JWKS cache-miss fetch) as well as
	// the handler's own DAO call, per decisions/context-propagation.md's
	// ruling — a single deadline, not the middleware's and the handler's
	// own disagreeing.
	protect := func(h http.Handler) http.Handler {
		return api.NewDeadlineMiddleware(authMiddleware(h))
	}

	var searchHandler, getHandler http.Handler
	if repo == nil {
		searchHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { api.WriteServiceUnavailable(w) })
		getHandler = searchHandler
	} else {
		deps := api.Deps{
			Repo:        repo,
			RateLimiter: api.NewInProcessRateLimiter(),
			TouchCounter: api.NewInProcessTouchCounter(
				api.TouchCapPerWindow, api.TouchCapWindow),
			AuditLog: auditLog,
			Authz: &api.StopgapAuthorizer{
				ReadOwnAllow: stopgapReadOwnAllow(cfg),
			},
		}
		searchHandler = api.NewSearchHandler(deps)
		getHandler = api.NewGetProfileHandler(deps)
	}

	mux.Handle("POST /profiles/search", protect(searchHandler))
	mux.Handle("GET /profiles/{id}", protect(getHandler))

	// LT-53: the demo console. A plain static handler — no auth
	// middleware, no rate limiter of its own — sitting on the exact
	// same mux as every other route this router serves, so there is no
	// special-cased bypass to build or to accidentally omit: it gets
	// exactly the same (lack of) treatment /healthz already has, not a
	// new exemption invented for this route. The page's own JS is what
	// drives the real, fully-enforced /profiles/search and
	// /profiles/{id} routes above (and /auth/token on the issuer,
	// same-origin via Theo Bergman's, 04, ingress routing) — those
	// calls hit the real rate limiters inside searchHandler/getHandler
	// exactly as any other caller's would.
	mux.Handle("GET /demo", demoHandler())

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
