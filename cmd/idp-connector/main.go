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
	"loginid-takehome/internal/connector"
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

	// No real ABC/XYZ vendor exists for this take-home (LT-41's own
	// non-goal) — IDP_ABC_BASE_URL unset (the common case in this
	// environment) wires the self-contained StubVendorClient instead,
	// seeded with one demo identity so the connector's full /auth ->
	// /identity flow is exercisable end to end without a real vendor. If
	// IDP_ABC_BASE_URL IS set, a real (or real-shaped) vendor is assumed
	// reachable over HTTPS and HTTPVendorClient is used instead — this
	// process does not silently fall back to the stub if that vendor
	// turns out to be unreachable at request time; NewHTTPVendorClient
	// only fails loudly at startup, on a non-https URL.
	var vendor connector.VendorClient
	if cfg.IDPABCBaseURL != "" {
		httpVendor, err := connector.NewHTTPVendorClient(cfg.IDPABCBaseURL, nil)
		if err != nil {
			log.Fatalf("idp-connector: configuring vendor client: %v", err)
		}
		vendor = httpVendor
		log.Printf("idp-connector: using HTTP vendor client at %s", cfg.IDPABCBaseURL)
	} else {
		stub := connector.NewStubVendorClient()
		stub.Seed("demo-user", "demo-password", connector.Identity{
			Name:          "Jane Demo",
			Phone:         "+15555550100",
			StreetAddress: "1 Demo Way",
			Locality:      "Demoville",
			Region:        "CA",
			PostalCode:    "94000",
			Country:       "US",
		})
		vendor = stub
		log.Printf("idp-connector: IDP_ABC_BASE_URL not set — using in-process stub vendor (LT-41 non-goal: no real vendor integration)")
	}

	log.Printf("idp-connector: listening on %s", addr)
	if err := http.ListenAndServe(addr, app.NewConnectorRouter(cfg, vendor)); err != nil {
		log.Fatalf("idp-connector: %v", err)
	}
}
