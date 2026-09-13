// jwtmiddleware.go is LT-40's core: verify the client-credentials JWT
// scheme from handoff-03-auth.md v3/v4 and populate AuthContext for
// LT-39's already-built handlers. This is the actual integration seam
// with S6's work, not new response-shape design — a verification failure
// uses the exact same writeUnauthorized this package's handlers already
// call for a missing AuthContext.
package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtAlgorithm is the one signing algorithm this verifier ever accepts.
// Passed explicitly to jwt.WithValidMethods on every parse — never left
// to the library's default, per handoff-03-auth.md v3's explicit
// instruction ("check your JWT library's default behavior — several
// popular libraries have shipped this vulnerability by not doing this by
// default") and RFC 8725 §3.1. This is what makes alg:none and an
// HMAC-with-the-public-key-as-secret downgrade both fail before the
// library ever calls KeySource — jwt/v5's WithValidMethods rejects a
// token whose header alg isn't in this allowlist ahead of signature
// verification, not as a secondary check after.
const jwtAlgorithm = "RS256"

// claims is this service's JWT claim set (handoff-03-auth.md v3's
// minimum: sub, scope, exp, iat, aud, plus iss and kid via the header).
// RegisteredClaims gives exp/iat/iss/aud validation for free from the
// library — this verifier still explicitly re-checks Audience below
// because jwt/v5's built-in audience check (via WithAudience) does not
// distinguish "no aud claim at all" from "aud present but empty," and a
// token with no audience claim must not silently pass as if it matched.
type claims struct {
	Scope string `json:"scope"`
	jwt.RegisteredClaims
}

// jwtVerifier is the collaborator NewJWTMiddleware wraps — an interface
// so a test can supply a fake instead of a real *JWKSCache doing network
// I/O, mirroring this package's existing test-double approach
// (decisions/test-double-strategy.md).
type jwtVerifier struct {
	keys     KeySource
	issuer   string
	audience string
}

// verify parses and fully validates tokenString, returning the populated
// AuthContext on success. Every failure returns a generic error — the
// specific reason (expired, wrong audience, bad signature, unknown kid,
// disallowed algorithm) is never distinguishable to the caller of this
// function beyond "verification failed," which is what makes
// writeUnauthorized's single, undifferentiated 401 body correct here
// without an extra translation step.
func (v *jwtVerifier) verify(ctx context.Context, tokenString string) (AuthContext, error) {
	var c claims
	token, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token header has no kid")
		}
		return v.keys.KeyForKid(ctx, kid)
	}, jwt.WithValidMethods([]string{jwtAlgorithm}), jwt.WithIssuer(v.issuer))
	if err != nil {
		return AuthContext{}, err
	}
	if !token.Valid {
		return AuthContext{}, errors.New("token invalid")
	}

	// Audience checked explicitly, not via jwt.WithAudience above: v5's
	// audience option treats an all-nil/absent Audience claim
	// inconsistently across versions in a way we don't want to depend
	// on — asserting it here directly means this check's behavior is
	// this file's own to guarantee, not the library's default to trust.
	if !hasAudience(c.Audience, v.audience) {
		return AuthContext{}, errors.New("token audience mismatch")
	}

	if c.Subject == "" {
		return AuthContext{}, errors.New("token has no sub claim")
	}

	scope, err := singleScope(c.Scope)
	if err != nil {
		return AuthContext{}, err
	}

	return AuthContext{Sub: c.Subject, Scope: scope}, nil
}

func hasAudience(aud jwt.ClaimStrings, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}

// singleScope requires the token's scope claim to name exactly one
// distinct recognized scope. handoff-03-auth.md v3 describes the claim
// as "space-delimited, values below" without describing how a handler
// disambiguates a multi-scope token per request, and AuthContext (LT-39)
// carries exactly one Scope by design ("handoff-03-auth.md v3 doesn't
// describe multi-scope tokens being disambiguated per-request, so one
// request carries exactly one Scope"). Rather than guess which of
// several granted scopes a request is "really" using — silently picking
// one is exactly the kind of authorization ambiguity this project's
// contracts have consistently ruled against — a token naming zero or
// more than one DISTINCT recognized scope is rejected outright. This
// matches the provisioning model in decisions/search-authz-scoping.md: a
// client is provisioned for the one use case its credential exists for.
//
// Deduplicated by distinct value, not by occurrence count: a malformed
// but unambiguous claim like "profile:search profile:search" names one
// scope repeated, not two different ones to disambiguate between, and
// rejecting it as "more than one" would contradict this function's own
// reasoning above (Oren Castellan, PR #30 review).
func singleScope(raw string) (Scope, error) {
	fields := strings.Fields(raw)
	found := make(map[Scope]bool, len(fields))
	for _, f := range fields {
		switch Scope(f) {
		case ScopeReadOwn, ScopeReadAny, ScopeSearch:
			found[Scope(f)] = true
		}
	}
	switch len(found) {
	case 0:
		return "", errors.New("token scope claim names no recognized scope")
	case 1:
		for s := range found {
			return s, nil
		}
		panic("unreachable")
	default:
		return "", errors.New("token scope claim names more than one distinct recognized scope, which this verifier does not disambiguate per request")
	}
}

// NewJWTMiddleware verifies every request's bearer token and populates
// AuthContext on success, per handoff-03-auth.md v3/v4: public-key-only
// verification (keys never includes signing-key material — enforced
// structurally, since KeySource's real implementation, JWKSCache, only
// ever holds *rsa.PublicKey values parsed from a JWKS response), pinned
// RS256, kid-based key selection supporting a rotation overlap window (a
// KeySource backed by the issuer's full published key set, not a single
// cached key), and issuer/audience checks. Verification failure returns
// 401 via writeUnauthorized — the same path LT-39's handlers already use
// for a missing AuthContext, not a new response shape — with one
// exception: a KeySource failing closed on exceeded JWKS staleness
// (ErrJWKSStale) logs AuditJWKSUnavailable first, since that's a
// token-infrastructure degradation, not an ordinary bad-credential
// attempt, and worth distinguishing in the audit trail (Tomasz Wrede, 02
// cold review). audit may be nil (tests that don't care about audit
// output); production wiring always passes a real AuditLogger.
func NewJWTMiddleware(keys KeySource, issuer, audience string, audit AuditLogger) func(http.Handler) http.Handler {
	v := &jwtVerifier{keys: keys, issuer: issuer, audience: audience}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			tokenString, ok := bearerToken(r)
			if !ok {
				logAuthnFailure(ctx, audit, nil)
				writeUnauthorized(w)
				return
			}
			ac, err := v.verify(ctx, tokenString)
			if err != nil {
				logAuthnFailure(ctx, audit, err)
				writeUnauthorized(w)
				return
			}
			r = r.WithContext(WithAuthContext(ctx, ac))
			next.ServeHTTP(w, r)
		})
	}
}

func logAuthnFailure(ctx context.Context, audit AuditLogger, err error) {
	if audit == nil {
		return
	}
	kind := AuditAuthnFailure
	// Both sentinels mean "verification is unavailable" (a
	// token-infrastructure problem), not "this specific token is
	// invalid" — ErrJWKSStale (a cache exists but aged past max
	// staleness) and ErrJWKSUnreachable (no cache exists yet at all,
	// the cold-start case) get the same distinct audit kind. A live
	// review finding caught this middleware previously only checking
	// ErrJWKSStale, so a cold-start JWKS failure (e.g. a Service port
	// mismatch discovered on the very first mint) was miscategorized as
	// an ordinary bad-credential attempt.
	if errors.Is(err, ErrJWKSStale) || errors.Is(err, ErrJWKSUnreachable) {
		kind = AuditJWKSUnavailable
	}
	audit.Log(ctx, AuditEvent{Kind: kind, Timestamp: time.Now().UTC()})
}

// bearerToken's scheme match is case-insensitive ("Bearer", "bearer",
// "BEARER" all accepted) per RFC 7235 §2.1, which defines the
// auth-scheme token as case-insensitive — a prior case-sensitive match
// would 401 a structurally valid request from any client that happened
// to send a lowercase scheme (Oren Castellan, PR #30 review).
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefixLen = len("Bearer ")
	if len(h) <= prefixLen || !strings.EqualFold(h[:prefixLen], "Bearer ") {
		return "", false
	}
	tok := strings.TrimSpace(h[prefixLen:])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// deadlineBudget is the context.WithTimeout duration this middleware
// applies to every request before it reaches a handler — the concrete
// number decisions/context-propagation.md's ruling required ("this gets
// a concrete number attached when [the middleware] is written, not left
// as 'some deadline'"). Sized to cover realistic p99 DAO latency
// (single-digit milliseconds for an indexed Postgres/CockroachDB point
// query or trigram search, per 05-data-ops/research-cockroachdb-postgres-semantics.md)
// plus this middleware's own JWKS verification path (a cache hit is
// sub-millisecond; a cache-miss fetch to the issuer's in-cluster Service
// is bounded by deadlineBudget itself, not a separate timeout, since it
// happens inside the same request). 5 seconds leaves an order of
// magnitude of headroom over expected p99 without ever handing the DAO
// layer a near-zero remaining deadline as a matter of routine operation.
const deadlineBudget = 5 * time.Second

// NewDeadlineMiddleware applies deadlineBudget to every request's
// context before it reaches a handler — the enforcement site
// decisions/context-propagation.md's ruling named ("a concrete number
// attached when [the middleware] is written"). Composed outermost (see
// router wiring), so the budget covers JWT verification (including a
// JWKS cache-miss fetch) and the handler's own DAO call under one
// deadline, not two independently-sized timeouts that could disagree.
func NewDeadlineMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), deadlineBudget)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
