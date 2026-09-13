package connector

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"loginid-takehome/internal/api"
)

// identityFailureFloor is /identity's uniform response-time floor across
// its three failure branches (no-token, bad-token, vendor-rejected) —
// Marcus Ilori's (02) ruling on Tobias Lindqvist's timing-oracle
// objection (refinement/LT-41.md), recorded in full in
// connector-security.md §4. ~100ms, well under any context.WithTimeout
// budget elsewhere in the chain (vendorCallTimeout is 10s) so it never
// competes with those deadlines. This narrows, but does not fully close,
// the gap between a local-only failure (microseconds) and a
// vendor-round-trip failure (tens-hundreds of ms) — full closure is an
// accepted non-goal per that ruling.
const identityFailureFloor = 100 * time.Millisecond

// identityErrorBody is the one JSON shape every /identity failure
// returns, regardless of which of the three branches produced it —
// content-identical by construction (a single writeIdentityFailure call
// site below), not by three call sites happening to independently agree.
type identityErrorBody struct {
	Error string `json:"error"`
}

// vendorLimiter is the interface these handlers depend on — satisfied
// directly by api.GrantLimiter (PR #30/#34's already-built component,
// reused here rather than rebuilt per LT-41's own non-goals: "this
// story's own in-process rate limiting is sufficient, matching the
// pattern already used for RateLimiter/TouchCounter/GrantLimiter").
// Declared locally so this package's own tests can supply a minimal fake
// satisfying only the calls actually made here, per
// decisions/test-double-strategy.md.
type vendorLimiter interface {
	Allow(ctx context.Context, clientID, sourceIP string) (bool, error)
	RecordFailure(ctx context.Context, clientID, sourceIP string)
	RecordSuccess(ctx context.Context, clientID, sourceIP string)
}

// Deps bundles NewAuthHandler/NewIdentityHandler's collaborators.
//
// IdentityLimiter is the second rate-limit dimension attack-tree leaf 5
// actually specifies ("per identity attempted, not just per client") —
// Limiter alone (keyed by calling-API-client sub + source IP) means one
// success anywhere resets that caller's whole consecutive-failure count,
// letting a compromised caller cycle through many stolen identities
// indefinitely (Tomasz Wrede's cold review, PR #40). May be nil in tests
// that don't exercise this dimension; NewConnectorRouter always
// constructs a real one.
type Deps struct {
	Vendor          VendorClient
	Limiter         vendorLimiter
	IdentityLimiter *identityRateLimiter
	AuditLog        api.AuditLogger
}

// identityGateAllow/RecordFailure/RecordSuccess are deps.IdentityLimiter's
// call sites, tolerant of a nil IdentityLimiter (tests that only care
// about the caller-level Limiter dimension can omit it — the identity
// dimension then simply isn't enforced, never a panic). identityParts is
// variadic so a multi-field identity (phone AND name) can be hashed as
// separate components rather than pre-joined with a printable separator
// — see identitylimiter.go's hashIdentity doc comment.
func identityGateAllow(ctx context.Context, l *identityRateLimiter, callerSub string, identityParts ...string) (bool, error) {
	if l == nil {
		return true, nil
	}
	return l.allow(ctx, callerSub, identityParts...)
}

func identityGateRecordFailure(ctx context.Context, l *identityRateLimiter, callerSub string, identityParts ...string) {
	if l == nil {
		return
	}
	l.recordFailure(ctx, callerSub, identityParts...)
}

func identityGateRecordSuccess(ctx context.Context, l *identityRateLimiter, callerSub string, identityParts ...string) {
	if l == nil {
		return
	}
	l.recordSuccess(ctx, callerSub, identityParts...)
}

func sourceIPFrom(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeIdentityFailure is the single call site for every /identity
// failure response — the one place that guarantees byte-identical
// content across the no-token, bad-token, and vendor-rejected branches
// (connector-security.md §1/F3), never three call sites independently
// trying to agree.
func writeIdentityFailure(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, identityErrorBody{Error: "identity_lookup_failed"})
}

// waitForFloor sleeps out the remainder of identityFailureFloor since
// start, if any — applied uniformly across /identity's three failure
// branches so none of them is distinguishable from the others by
// response latency alone (partial mitigation per connector-security.md
// §4's own accepted-residual language; not applied to the success path,
// which was never part of the timing-oracle concern).
func waitForFloor(start time.Time) {
	if remaining := identityFailureFloor - time.Since(start); remaining > 0 {
		time.Sleep(remaining)
	}
}

// authRequestBody is the assignment's own fixed POST /auth contract.
type authRequestBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponseBody struct {
	AccessToken string `json:"access_token"`
}

// NewAuthHandler implements POST /auth. The caller (in this project's
// shape, cmd/api-service acting on an onboarding flow's behalf) must
// already be authenticated with a connector:identity-lookup-scoped,
// idp-connector-service-audience token — enforced by whatever middleware
// wraps this handler (internal/app.NewConnectorRouter), not by this
// function; AuthContextFromContext here is read-only identification for
// rate-limiting and audit purposes.
func NewAuthHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ac, _ := api.AuthContextFromContext(ctx)
		sourceIP := sourceIPFrom(r)

		allowed, err := deps.Limiter.Allow(ctx, ac.Sub, sourceIP)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, identityErrorBody{Error: "server_error"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusTooManyRequests, identityErrorBody{Error: "slow_down"})
			return
		}

		var body authRequestBody
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, identityErrorBody{Error: "invalid_request"})
			return
		}

		// Second rate-limit dimension — per (caller, attempted vendor
		// username), not just per caller (attack-tree leaf 5, Tomasz
		// Wrede's cold review of PR #40): gates BEFORE the vendor call,
		// same as the caller-level Limiter above.
		identityAllowed, err := identityGateAllow(ctx, deps.IdentityLimiter, ac.Sub, body.Username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, identityErrorBody{Error: "server_error"})
			return
		}
		if !identityAllowed {
			writeJSON(w, http.StatusTooManyRequests, identityErrorBody{Error: "slow_down"})
			return
		}

		// accessToken is scoped to this block only — read from the
		// vendor call's return value, written straight into the
		// response, never assigned to any wider-scoped variable or
		// logged. This IS the narrowest this value can be scoped given
		// /auth's own contract: its job is delivering the token to the
		// caller, so it necessarily reaches the response writer, but
		// nothing else.
		accessToken, err := deps.Vendor.Authenticate(ctx, body.Username, body.Password)
		if err != nil {
			deps.Limiter.RecordFailure(ctx, ac.Sub, sourceIP)
			identityGateRecordFailure(ctx, deps.IdentityLimiter, ac.Sub, body.Username)
			logAuditOutcome(ctx, deps.AuditLog, ac.Sub, api.AuditAuthnFailure)
			// Generic failure regardless of *why* the vendor rejected
			// it (unknown username vs. wrong password vs. vendor
			// unreachable) — connector-security.md §4: "must not leak
			// which part of the credential was wrong."
			writeJSON(w, http.StatusUnauthorized, identityErrorBody{Error: "vendor_authentication_failed"})
			return
		}
		deps.Limiter.RecordSuccess(ctx, ac.Sub, sourceIP)
		identityGateRecordSuccess(ctx, deps.IdentityLimiter, ac.Sub, body.Username)
		logAuditOutcome(ctx, deps.AuditLog, ac.Sub, api.AuditSuccess)

		writeJSON(w, http.StatusOK, authResponseBody{AccessToken: accessToken})
	}
}

// identityRequestBody is the assignment's own fixed POST /identity
// contract — no credential field, since the vendor token travels as a
// request header instead (connector-security.md §1).
type identityRequestBody struct {
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

type identityResponseBody struct {
	Name          string `json:"name"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	Locality      string `json:"locality"`
	Region        string `json:"region"`
	PostalCode    string `json:"postal_code"`
	Country       string `json:"country"`
}

// vendorAccessTokenHeader is the header /identity accepts the vendor's
// access_token on — a header, not a body field, since the assignment's
// fixed {"phone","name"} body has no credential field (LT-41 acceptance
// criteria).
const vendorAccessTokenHeader = "X-Vendor-Access-Token"

// NewIdentityHandler implements POST /identity. Every failure path
// (missing token, malformed-looking token, vendor rejection) returns
// through writeIdentityFailure after waitForFloor — the two calls
// together are what make the three branches content- and
// (approximately) time-identical, per connector-security.md §1/§4.
func NewIdentityHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := r.Context()
		ac, _ := api.AuthContextFromContext(ctx)
		sourceIP := sourceIPFrom(r)

		allowed, err := deps.Limiter.Allow(ctx, ac.Sub, sourceIP)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, identityErrorBody{Error: "server_error"})
			return
		}
		if !allowed {
			// Not one of the three timing-oracle branches (it's an
			// overt, openly-disclosed 429, not a covert signal) — no
			// floor applied.
			writeJSON(w, http.StatusTooManyRequests, identityErrorBody{Error: "slow_down"})
			return
		}

		var body identityRequestBody
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			// A malformed body is a caller bug, not one of the three
			// named credential/token-validity branches — ordinary 400,
			// no floor.
			writeJSON(w, http.StatusBadRequest, identityErrorBody{Error: "invalid_request"})
			return
		}

		// Second rate-limit dimension — per (caller, attempted
		// phone+name), not just per caller (attack-tree leaf 5, Tomasz
		// Wrede's cold review of PR #40). Not one of the three
		// timing-oracle branches (an overt 429, same as the caller-level
		// gate above) — no floor.
		attemptedIdentity := []string{body.Phone, body.Name} // hashed as separate components, never pre-joined (Oren Castellan, PR #40 review)
		identityAllowed, err := identityGateAllow(ctx, deps.IdentityLimiter, ac.Sub, attemptedIdentity...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, identityErrorBody{Error: "server_error"})
			return
		}
		if !identityAllowed {
			writeJSON(w, http.StatusTooManyRequests, identityErrorBody{Error: "slow_down"})
			return
		}

		token := strings.TrimSpace(r.Header.Get(vendorAccessTokenHeader))
		if token == "" {
			deps.Limiter.RecordFailure(ctx, ac.Sub, sourceIP)
			identityGateRecordFailure(ctx, deps.IdentityLimiter, ac.Sub, attemptedIdentity...)
			logAuditOutcome(ctx, deps.AuditLog, ac.Sub, api.AuditAuthnFailure)
			waitForFloor(start)
			writeIdentityFailure(w)
			return
		}

		// token is scoped to this function only, passed by value into
		// FetchIdentity, and never assigned to any variable outside
		// this block or returned — attack-tree leaf 4's control
		// (connector-security.md §1/Attack-tree row 4).
		identity, err := deps.Vendor.FetchIdentity(ctx, token, body.Phone, body.Name)
		if err != nil {
			deps.Limiter.RecordFailure(ctx, ac.Sub, sourceIP)
			identityGateRecordFailure(ctx, deps.IdentityLimiter, ac.Sub, attemptedIdentity...)
			logAuditOutcome(ctx, deps.AuditLog, ac.Sub, api.AuditAuthnFailure)
			waitForFloor(start)
			writeIdentityFailure(w)
			return
		}
		deps.Limiter.RecordSuccess(ctx, ac.Sub, sourceIP)
		identityGateRecordSuccess(ctx, deps.IdentityLimiter, ac.Sub, attemptedIdentity...)
		logAuditOutcome(ctx, deps.AuditLog, ac.Sub, api.AuditSuccess)

		// Success is not part of the timing-oracle concern — no floor.
		// identityResponseBody's fields are identical in name/order/type
		// to Identity (only the json tags differ), so a direct type
		// conversion is both valid and clearer than restating every
		// field (staticcheck S1016).
		writeJSON(w, http.StatusOK, identityResponseBody(identity))
	}
}

// logAuditOutcome logs caller sub/scope/timestamp/outcome only — never
// the vendor username/password, vendor access_token, or any PII field
// value, per connector-security.md §3/§5's audit discipline.
func logAuditOutcome(ctx context.Context, auditLog api.AuditLogger, sub string, kind api.AuditEventKind) {
	if auditLog == nil {
		return
	}
	auditLog.Log(ctx, api.AuditEvent{
		Sub:       sub,
		Scope:     api.ScopeConnectorIdentityLookup,
		Kind:      kind,
		Timestamp: time.Now().UTC(),
	})
}
