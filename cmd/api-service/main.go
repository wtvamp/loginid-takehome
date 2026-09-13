// Command api-service is the client-facing binary (Q1 DAO + Q2 REST API,
// bearer-token auth). Per decisions/go-layout-debate.md, this file is a
// thin wrapper — route registration and config loading live in
// internal/app and internal/config, not here.
package main

import (
	"log"
	"net/http"

	"loginid-takehome/internal/app"
	"loginid-takehome/internal/config"
	"loginid-takehome/internal/dao"

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
		repo, err := dao.New(cfg.DBDriver, cfg.DBDSN)
		if err != nil {
			log.Fatalf("api-service: opening DAO repository: %v", err)
		}
		handler = app.NewVerifierRouter(cfg, repo)
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
