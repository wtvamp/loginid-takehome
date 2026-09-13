// Command api-service is the client-facing binary (Q1 DAO + Q2 REST API,
// bearer-token auth). Per decisions/go-layout-debate.md, this file is a
// thin wrapper — route registration and config loading live in
// internal/app and internal/config, not here.
package main

import (
	"database/sql"
	"log"
	"net/http"

	"loginid-takehome/internal/app"
	"loginid-takehome/internal/config"
	"loginid-takehome/internal/dao"

	_ "loginid-takehome/internal/dao/postgres" // registers "postgres" and "cockroachdb"
	_ "loginid-takehome/internal/dao/sqlite"   // registers "sqlite"
)

// sqlDriverNameFor maps cfg.DBDriver ("postgres"/"cockroachdb"/"sqlite" —
// dao.Register's driver names, per dao/factory.go) to the underlying
// database/sql driver name each backend package registers itself under
// (internal/dao/postgres/postgres.go: "pgx"; internal/dao/sqlite/sqlite.go:
// "sqlite") — needed only for LT-52's separate /healthz ping handle, since
// dao.Repository itself exposes no Ping and this is a thin sql.Open, not a
// second copy of either backend's connection logic.
func sqlDriverNameFor(daoDriver string) string {
	switch daoDriver {
	case "postgres", "cockroachdb":
		return "pgx"
	case "sqlite":
		return "sqlite"
	default:
		return ""
	}
}

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

		// A second, independent *sql.DB purely for /healthz's bounded
		// connectivity probe (LT-52 criterion 5) — sql.Open never dials,
		// it only validates the driver/DSN, so this never blocks
		// startup; PingContext at request time is what actually checks
		// connectivity, bounded to dbPingTimeout. Deliberately separate
		// from repo above rather than routed through dao.Repository,
		// which 05's contract doesn't expose a Ping through.
		var pingDB *sql.DB
		if driverName := sqlDriverNameFor(cfg.DBDriver); driverName != "" {
			pingDB, err = sql.Open(driverName, cfg.DBDSN)
			if err != nil {
				log.Printf("api-service: opening /healthz DB ping handle: %v — /healthz will report db=down", err)
				pingDB = nil
			}
		}

		handler = app.NewVerifierRouter(cfg, repo, pingDB)
	default:
		// ModeIssuer (and the unrecognized-mode fallback, which
		// NewRouter itself already logs loudly and treats as verifier —
		// see app.NewRouter's own doc comment) don't need the full
		// verifier stack; NewRouter's existing /auth/token placeholder
		// (LT-32) and /healthz cover both.
		handler = app.NewRouter("api-service", app.Mode(cfg.AppMode))
	}

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("api-service: %v", err)
	}
}
