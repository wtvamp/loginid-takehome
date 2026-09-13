package app

import (
	"log"
	"net/http"
	"time"

	"loginid-takehome/internal/api"
	"loginid-takehome/internal/config"
	"loginid-takehome/internal/connector"
)

// connectorJWKSCacheTTL matches api-service's own verifier JWKS TTL
// (internal/app/verifier_router.go) — both binaries verify tokens minted
// by the same issuer, so there's no reason for a different refresh
// cadence.
const connectorJWKSCacheTTL = 15 * time.Minute

// connectorVendorLimiterBaseBackoff/MaxBackoff/AlertThreshold mirror
// NewIssuerRouter's own GrantLimiter constants — same reasoning
// (handoff-03-auth.md names the mechanism, not specific numbers), reused
// here for the vendor-call rate limiter per LT-41's attack-tree leaf 5
// control rather than inventing a second set of magic numbers.
const (
	connectorVendorLimiterBaseBackoff    = 1 * time.Second
	connectorVendorLimiterMaxBackoff     = 5 * time.Minute
	connectorVendorLimiterAlertThreshold = 5
)

// NewConnectorRouter builds cmd/idp-connector's real router — LT-41's
// /auth and /identity handlers behind the connector's own inbound JWT
// verification (a distinct audience, connector-security.md §5: aud =
// cfg.ConnectorJWTAudience, scope = connector:identity-lookup — NOT
// api-service's audience, so a profile:*-scoped api-service token
// cannot be replayed against this connector and vice versa).
//
// vendor is the outbound seam to the third-party IDP — either a real
// (HTTPVendorClient) or stub (StubVendorClient) implementation;
// cmd/idp-connector/main.go decides which based on whether
// IDP_ABC_BASE_URL is configured.
func NewConnectorRouter(cfg config.Config, keys api.KeySource, vendor connector.VendorClient) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler("idp-connector"))

	auditLog := api.StdoutAuditLogger{}
	authMiddleware := api.NewJWTMiddleware(keys, cfg.AuthJWTIssuer, cfg.ConnectorJWTAudience, auditLog)
	protect := func(h http.Handler) http.Handler {
		return api.NewDeadlineMiddleware(authMiddleware(h))
	}

	// One shared limiter instance across both handlers — a caller
	// hammering /auth and a caller hammering /identity are the same
	// abuse signal against the same vendor, keyed by the same
	// (calling-API-client sub, source IP) pair (LT-41 attack-tree
	// leaf 5). Reuses api.GrantLimiter (PR #30/#34) rather than
	// building a new rate-limit type, per this story's own non-goals.
	limiter := api.NewInProcessGrantLimiter(
		connectorVendorLimiterBaseBackoff, connectorVendorLimiterMaxBackoff, connectorVendorLimiterAlertThreshold,
		func(clientID string, consecutiveFailures int) {
			log.Printf("idp-connector: ALERT sustained failed vendor calls from caller sub=%s (%d consecutive)", clientID, consecutiveFailures)
		},
	)

	deps := connector.Deps{Vendor: vendor, Limiter: limiter, AuditLog: auditLog}
	mux.Handle("POST /auth", protect(connector.NewAuthHandler(deps)))
	mux.Handle("POST /identity", protect(connector.NewIdentityHandler(deps)))

	return mux
}
