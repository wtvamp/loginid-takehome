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
	if err := http.ListenAndServe(addr, app.NewRouter("api-service", cfg.AppMode)); err != nil {
		log.Fatalf("api-service: %v", err)
	}
}
