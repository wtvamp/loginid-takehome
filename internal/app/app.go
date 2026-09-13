// Package app is the shared bootstrap both cmd/api-service and
// cmd/idp-connector wrap thinly, per decisions/go-layout-debate.md's ruling:
// route registration and config wiring live once here, not duplicated per
// binary. It carries no behavior beyond wiring — service-specific handlers
// belong in internal/api and internal/connector as those stories land.
package app

import (
	"encoding/json"
	"net/http"
)

// healthResponse is the JSON body every health probe returns. Field set is
// deliberately minimal per 02's ruling (api-auth-design.md) on this
// unauthenticated endpoint: an up/down status, which service answered, and
// an opaque build identifier (commit SHA) — never a version string, build
// timestamp, connection detail, or anything else. Do not add fields here
// without checking that ruling first; see refinement/LT-34.md.
type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Build   string `json:"build"`
}

// ModeVerifier and ModeIssuer are api-service's two run modes (LT-32),
// selected by config.Config.AppMode and passed to NewRouter by cmd/
// api-service/main.go. idp-connector has no issuer/verifier split — its
// main.go passes ModeVerifier, the only mode that adds no route beyond
// /healthz, since that value is also this package's harmless default.
const (
	ModeVerifier = "verifier"
	ModeIssuer   = "issuer"
)

// NewRouter builds the http.Handler shared by both binaries' main.go.
// service identifies which binary is answering ("api-service" or
// "idp-connector") in the health response, per LT-34's acceptance
// criteria. mode is api-service's APP_MODE (LT-32) — the router itself is
// this story's startup-time branch point named in refinement/LT-32.md's
// acceptance criteria ("internal/app branches on it at startup"): in
// ModeIssuer, a placeholder route is registered at the path reserved for
// token issuance so the issuer and verifier Deployments are demonstrably
// different routers, not just different env vars read by code that
// otherwise behaves identically. The actual issuance handler is S7/LT-40's
// scope, not this story's (refinement/LT-32.md's non-goals) — this only
// proves the wiring exists and is reachable on the Deployment that is
// supposed to have it, and absent on the one that isn't.
func NewRouter(service string, mode string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler(service))
	if mode == ModeIssuer {
		mux.HandleFunc("/auth/token", issuerPlaceholderHandler)
	}
	return mux
}

// issuerPlaceholderHandler exists only to make the issuer/verifier router
// split observable and testable before S7/LT-40 implements real token
// issuance. It deliberately does no cryptography and holds no reference to
// config.Config.JWTSigningKeyPath — that wiring belongs to whichever S7
// handler replaces this one.
func issuerPlaceholderHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "token issuance not yet implemented — reserved for LT-40", http.StatusNotImplemented)
}

func healthHandler(service string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status:  "ok",
			Service: service,
			Build:   Commit,
		})
	}
}
