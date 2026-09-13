// tokenhandler.go implements POST /auth/token — RFC 6749 §4.4's
// client-credentials grant. This is the one endpoint in the API that
// follows OAuth2's own wire format (form-encoded, HTTP Basic client
// auth) rather than this project's usual JSON convention, because it's
// the one endpoint implementing a named external grant type — and, per
// Tobias Lindqvist's objection (LT-51 refinement), the endpoint most
// likely to be pattern-matched against known real-world OAuth2
// timing-oracle bugs by an outside reviewer.
package api

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// tokenTTL is this issuer's fixed token lifetime — the middle of
// handoff-03-auth.md's 5-15 minute range. No refresh tokens: a client
// re-authenticates with its own client secret on expiry, per
// api-auth-design.md's standing TTL-only-revocation design (this
// story's own non-goal list; not re-litigated here).
const tokenTTL = 10 * time.Minute

// tokenErrorResponse is RFC 6749 §5.2's error body shape.
type tokenErrorResponse struct {
	Error string `json:"error"`
}

// tokenResponse is RFC 6749 §5.1's success body shape.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func writeTokenError(w http.ResponseWriter, status int, errCode string) {
	if status == http.StatusUnauthorized {
		// RFC 6749 §5.2: invalid_client with a 401 MUST carry
		// WWW-Authenticate identifying the auth scheme this endpoint
		// expects — the same header any HTTP Basic-protected resource
		// returns, not something specific to OAuth2 error semantics.
		w.Header().Set("WWW-Authenticate", `Basic realm="token"`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncode(w, tokenErrorResponse{Error: errCode})
}

// jsonEncode is a one-line indirection kept trivial and named for
// clarity rather than inlined at each of this file's two call sites.
func jsonEncode(w http.ResponseWriter, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// NewTokenHandler implements POST /auth/token, wiring the collaborators
// named in refinement/LT-51.md's explicit instruction rather than
// reimplementing them: store is the client datastore (this story's own,
// new); limiter is PR #30's already-built GrantLimiter, called exactly
// as its own doc comment specifies — Allow gates before authentication,
// RecordFailure/RecordSuccess run after. The signing key parameter holds
// the loaded RSA private key this story mints tokens with (see
// SigningKey in signingkey.go).
func NewTokenHandler(store ClientStore, limiter GrantLimiter, key *SigningKey, issuer string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// store is nil when the issuer database was never configured
		// (config.Config.IssuerDBDSN empty) — fail every grant closed
		// with server_error rather than panic on a nil ClientStore, the
		// same fail-closed shape NewVerifierRouter uses for a nil
		// dao.Repository. This deliberately runs before ParseForm/
		// BasicAuth so a misconfigured issuer fails identically for
		// every request shape, not just the ones that get far enough to
		// reach store.Get.
		if store == nil {
			writeTokenError(w, http.StatusInternalServerError, "server_error")
			return
		}

		// RFC 6749 §4.4: form-encoded, not JSON. r.ParseForm reads the
		// body as application/x-www-form-urlencoded regardless of a
		// caller's declared Content-Type, which is deliberately
		// permissive here — this handler's job is accepting the one wire
		// format the spec names, not policing a header a legitimate
		// client-credentials implementation might send inconsistently.
		if err := r.ParseForm(); err != nil {
			writeTokenError(w, http.StatusBadRequest, "invalid_request")
			return
		}
		if r.PostForm.Get("grant_type") != "client_credentials" {
			writeTokenError(w, http.StatusBadRequest, "unsupported_grant_type")
			return
		}

		clientID, clientSecret, hasBasicAuth := r.BasicAuth()
		sourceIP := sourceIPFrom(r)

		// GrantLimiter gates BEFORE the hasBasicAuth check, not after —
		// sourceIP is available regardless of whether a caller sent any
		// credentials at all, and a request with no Authorization header
		// is exactly as capable of flooding this endpoint as one with a
		// wrong client_id (Helena Marsh, 02 review of PR #34). clientID
		// is "" on this path; GrantLimiter treats it as an ordinary
		// (empty) key like any other, and the per-source-IP half of the
		// gate is what actually matters here since there's no client_id
		// yet to distinguish attempts by.
		allowed, err := limiter.Allow(ctx, clientID, sourceIP)
		if err != nil {
			writeTokenError(w, http.StatusInternalServerError, "server_error")
			return
		}
		if !allowed {
			// RFC 6749 doesn't name a status for endpoint-level
			// throttling (it's a brute-force control this project adds
			// on top of the spec, not part of it) — 429 matches this
			// API's own convention elsewhere (internal/api/ratelimit.go,
			// internal/api/touchcounter.go) for the same class of
			// decision.
			writeTokenError(w, http.StatusTooManyRequests, "slow_down")
			return
		}

		if !hasBasicAuth {
			// No Authorization header at all: reject before ever
			// touching the client store — there is no client_id to key
			// a store lookup by. RecordFailure still fires (keyed by
			// sourceIP; clientID is "") so repeated no-credential
			// requests still advance that source IP's backoff, same as
			// any other failed attempt.
			limiter.RecordFailure(ctx, clientID, sourceIP)
			writeTokenError(w, http.StatusUnauthorized, "invalid_client")
			return
		}

		rec, found, err := store.Get(ctx, clientID)
		if err != nil {
			writeTokenError(w, http.StatusInternalServerError, "server_error")
			return
		}

		// Both branches run the SAME Argon2id comparison at the SAME
		// cost before deciding anything — a found client with a real
		// secret on file (secret_state = "set") compares against its own
		// hash; every other case (client_id not found at all, OR found
		// but secret_state is "none"/"revoked" and ClientSecretHash is
		// empty) compares against the fixed dummyHash instead of
		// skipping the comparison or short-circuiting on a malformed
		// hash. Whichever branch runs, exactly one verifySecret call
		// happens before the outcome is known, so an unknown client_id,
		// a not-yet-secreted or revoked client, and a wrong secret for a
		// fully provisioned one all take statistically indistinguishable
		// time (Tobias Lindqvist's objection, LT-51 refinement) —
		// closing the timing side-channel the identical response body
		// alone leaves open.
		var secretOK bool
		if found && rec.ClientSecretHash != "" {
			secretOK = verifySecret(clientSecret, rec.ClientSecretHash)
		} else {
			_ = verifySecret(clientSecret, dummyHash)
			secretOK = false
		}

		if !found || !secretOK || !rec.Active {
			limiter.RecordFailure(ctx, clientID, sourceIP)
			writeTokenError(w, http.StatusUnauthorized, "invalid_client")
			return
		}
		limiter.RecordSuccess(ctx, clientID, sourceIP)

		// scope is optional per RFC 6749 §4.4: omitted means issue the
		// client's full granted set, which — per ClientRecord's own doc
		// comment on why GrantedScope is singular, not a set — is
		// exactly one scope. A requested scope must equal that one
		// scope exactly; anything else (a different scope, an empty
		// string when one is explicitly passed, more than one
		// space-delimited value) is invalid_scope. This is stricter than
		// "subset of a set" only because the set this system provisions
		// is never larger than one element to begin with.
		requested := strings.TrimSpace(r.PostForm.Get("scope"))
		if requested != "" && Scope(requested) != rec.GrantedScope {
			writeTokenError(w, http.StatusBadRequest, "invalid_scope")
			return
		}

		now := time.Now()
		c := claims{
			Scope: string(rec.GrantedScope),
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    issuer,
				Subject:   clientID,
				Audience:  jwt.ClaimStrings{rec.Audience},
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
				// jti: audit-correlation aid only, per handoff-03-auth.md
				// v4 — never checked against a denylist at verification
				// time (this design's TTL-only revocation stands).
				ID: uuid.NewString(),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
		token.Header["kid"] = key.Kid
		signed, err := token.SignedString(key.Private)
		if err != nil {
			writeTokenError(w, http.StatusInternalServerError, "server_error")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store") // RFC 6749 §5.1: token responses must not be cached
		w.WriteHeader(http.StatusOK)
		_ = jsonEncode(w, tokenResponse{
			AccessToken: signed,
			TokenType:   "Bearer",
			ExpiresIn:   int(tokenTTL.Seconds()),
			Scope:       string(rec.GrantedScope),
		})
	}
}

// sourceIPFrom extracts the caller's address for GrantLimiter's
// per-source-IP key. r.RemoteAddr is the direct TCP peer — if this
// service ever sits behind a proxy that needs X-Forwarded-For honored
// instead, that's a deployment-topology decision for whoever wires the
// proxy, not assumed here.
func sourceIPFrom(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
