package api

import (
	"encoding/json"
	"net/http"
)

// jwksCacheControlMaxAge is the advisory Cache-Control this endpoint
// sends. handoff-03-auth.md v6 is explicit that this is advisory only —
// the verifier's own staleness policy (JWKSCache: max staleness = 3x its
// refresh TTL, fail closed past that, per-kid rate-limited refetch on an
// unknown kid) is authoritative and does not read or depend on this
// header. Set to a conservative value well under the verifier's own
// 15-minute refresh TTL purely as good HTTP citizenship for any other
// cache that might sit between them.
const jwksCacheControlMaxAge = "max-age=300"

// NewJWKSHandler serves key's public component at the now-fixed path
// /.well-known/jwks.json (handoff-03-auth.md v6, OIDC-discovery
// convention) — issuer-mode only, reachable in-cluster via the issuer's
// own ClusterIP Service (04's manifest side; this handler has no
// opinion on Ingress routing, it just serves the response wherever it's
// mounted). The response shape is exactly what internal/api/jwks.go's
// parseJWKS expects on the verifier side — the same RFC 7517 document
// shape, produced by SigningKey.JWKS().
func NewJWKSHandler(key *SigningKey) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, "+jwksCacheControlMaxAge)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(key.JWKS())
	}
}
