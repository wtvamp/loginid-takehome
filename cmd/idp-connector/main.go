// Command idp-connector is the outbound-only binary (Q3: /auth, /identity
// against third-party IDPs). Per decisions/go-layout-debate.md, this file
// is a thin wrapper — route registration and config loading live in
// internal/app and internal/config, not here.
package main

import (
	"log"
	"net/http"

	"loginid-takehome/internal/app"
	"loginid-takehome/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("idp-connector: loading config: %v", err)
	}

	addr := cfg.HTTPAddr
	if addr == "" {
		addr = ":8081"
	}

	log.Printf("idp-connector: listening on %s", addr)
	if err := http.ListenAndServe(addr, app.NewRouter("idp-connector")); err != nil {
		log.Fatalf("idp-connector: %v", err)
	}
}
