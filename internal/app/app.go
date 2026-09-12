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

// NewRouter builds the http.Handler shared by both binaries' main.go. service
// identifies which binary is answering ("api-service" or "idp-connector") in
// the health response, per LT-34's acceptance criteria.
func NewRouter(service string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler(service))
	return mux
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
