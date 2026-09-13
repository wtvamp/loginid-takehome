// Command api-service is the client-facing binary (Q1 DAO + Q2 REST API,
// bearer-token auth). Per decisions/go-layout-debate.md, this file is a
// thin wrapper — route registration and config loading live in
// internal/app and internal/config, not here.
package main

import (
	"database/sql"
	"log"
	"net/http"

	"loginid-takehome/internal/api"
	"loginid-takehome/internal/app"
	"loginid-takehome/internal/config"
	"loginid-takehome/internal/dao"

	// internal/dao/postgres's own blank import of pgx/v5/stdlib already
	// registers the "pgx" database/sql driver this file uses directly
	// for the issuer database below — Go dedupes an import by package
	// path, so this isn't a second registration, and no separate blank
	// import of that package is needed here.
	_ "loginid-takehome/internal/dao/postgres" // registers "postgres" and "cockroachdb"
	_ "loginid-takehome/internal/dao/sqlite"   // registers "sqlite"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("api-service: loading config: %v", err)
	}

	addr := cfg.HTTPAddr
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("api-service: mode=%s listening on %s", cfg.AppMode, addr)

	var handler http.Handler
	switch app.Mode(cfg.AppMode) {
	case app.ModeVerifier:
		// The verifying Deployment is the one that talks to the
		// database (LT-39's search/retrieve handlers) — the issuer
		// Deployment (below) never needs a Repository at all, since it
		// only mints tokens.
		//
		// A failure here does NOT Fatal: on a cluster where the database
		// doesn't exist yet (no DB_DRIVER/DB_DSN configured — e.g. before
		// PR #29's Postgres StatefulSet lands), or DB_DRIVER is simply
		// misconfigured, this process must still come up and serve
		// /healthz rather than crash-loop the whole verifying Deployment
		// over a dependency only two of its routes need. repo stays nil;
		// NewVerifierRouter fails those two routes closed with 503
		// instead of panicking on a nil Repository.
		repo, err := dao.New(cfg.DBDriver, cfg.DBDSN)
		if err != nil {
			log.Printf("api-service: opening DAO repository: %v — /profiles/search and /profiles/{id} will return 503 until this is resolved", err)
			repo = nil
		}
		handler = app.NewVerifierRouter(cfg, repo)
	case app.ModeIssuer:
		// config.Load already fails startup if JWT_SIGNING_KEY_FILE is
		// missing/unreadable in issuer mode (resolveSigningKeyPath) —
		// LoadSigningKey failing here means the file is readable but not
		// a valid RSA private key, a genuine misconfiguration this
		// process cannot usefully run past (there is no "serve /healthz,
		// fail closed" story for a router with no key at all: both of
		// its routes need one). Fatal, not degrade — unlike the DAO
		// repository above, which has a real story for "not there yet."
		key, err := api.LoadSigningKey(cfg.JWTSigningKeyPath)
		if err != nil {
			log.Fatalf("api-service: loading JWT signing key: %v", err)
		}

		// issuerDB, like the verifier's pingDB, is opened independently
		// of whether it can be reached yet — sql.Open never dials. Empty
		// IssuerDBDSN (not configured yet) leaves issuerDB nil;
		// NewIssuerRouter's /auth/token then fails every grant closed
		// with server_error rather than panicking on a nil ClientStore.
		var issuerDB *sql.DB
		if cfg.IssuerDBDSN != "" {
			issuerDB, err = sql.Open("pgx", cfg.IssuerDBDSN)
			if err != nil {
				log.Printf("api-service: opening issuer database: %v — /auth/token will return server_error until this is resolved", err)
				issuerDB = nil
			}
		} else {
			log.Printf("api-service: ISSUER_DB_DSN/ISSUER_DB_DSN_FILE not set — /auth/token will return server_error until this is resolved")
		}

		handler = app.NewIssuerRouter(cfg, key, issuerDB)
	default:
		// The unrecognized-mode fallback (NewRouter itself already logs
		// loudly and treats it as verifier — see app.NewRouter's own doc
		// comment) doesn't need the full verifier stack; NewRouter's
		// /healthz alone covers it, matching the safer of the two route
		// sets.
		handler = app.NewRouter("api-service", app.Mode(cfg.AppMode))
	}

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("api-service: %v", err)
	}
}
