// Package api implements the Q2 REST handlers (search/retrieve
// user_profile) per 02-ai-security-architecture/handoff-03-auth.md v3 and
// 03-engineering-delivery/refinement/LT-39.md. This story's own non-goals
// (LT-39, S6) exclude the JWT verification middleware itself — that's S7
// (LT-40), a separate pull — so every handler here reads its auth context
// from the request's context.Context, populated by whatever put it there.
// Until S7 lands, that's this package's own tests using fakes, per
// decisions/test-double-strategy.md; once S7 lands, it's S7's middleware.
// These handlers are not yet wired into cmd/api-service's live router —
// doing so before S7 exists would expose unauthenticated PII, which is
// exactly the outcome the whole auth design exists to prevent.
package api

import "context"

// Scope is the token's authorized action, from handoff-03-auth.md v3's
// closed vocabulary (§ "Scope vocabulary"). A typed string rather than a
// bare one so a handler comparing scopes can't typo past the compiler on
// a scope it never validates against a literal.
type Scope string

const (
	// ScopeReadOwn retrieves a single record by id the caller already
	// holds a legitimate reference to; authorize() checks the target
	// against the caller_referral mapping.
	ScopeReadOwn Scope = "profile:read:own"
	// ScopeReadAny retrieves a single record by id with no referral
	// constraint, but requires a ReasonCode on every request and is
	// still subject to the cumulative touch cap.
	ScopeReadAny Scope = "profile:read:any"
	// ScopeSearch queries by name/phone fragment across the table. The
	// broadest scope; there is no broader one.
	ScopeSearch Scope = "profile:search"
)

// AuthContext is what a validated request carries once past S7's
// middleware (or, until S7 exists, whatever a test injects directly via
// WithAuthContext). Sub is the calling client identity (the token's `sub`
// claim); Scope is the single scope this request is being made under —
// handoff-03-auth.md v3 doesn't describe multi-scope tokens being
// disambiguated per-request, so one request carries exactly one Scope.
type AuthContext struct {
	Sub   string
	Scope Scope
}

type authContextKey struct{}

// WithAuthContext attaches ac to ctx. The seam S7's middleware (or a
// test) uses to populate the auth context every handler in this package
// reads.
func WithAuthContext(ctx context.Context, ac AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, ac)
}

// AuthContextFromContext retrieves the AuthContext WithAuthContext
// attached, ok=false if none was ever set (e.g. this package's handlers
// were reached without going through auth middleware — a caller bug, not
// a request the handler should try to interpret).
func AuthContextFromContext(ctx context.Context) (AuthContext, bool) {
	ac, ok := ctx.Value(authContextKey{}).(AuthContext)
	return ac, ok
}

// ReasonCode is the closed enum profile:read:any requires on every
// request (handoff-03-auth.md v3). Adding a value is a documented change
// to this list, not a runtime config toggle.
type ReasonCode string

const (
	ReasonSupportTicket     ReasonCode = "support_ticket"
	ReasonFraudReview       ReasonCode = "fraud_review"
	ReasonKYCReverification ReasonCode = "kyc_reverification"
	ReasonLegalHold         ReasonCode = "legal_hold"
)

// Valid reports whether r is one of the four closed enum values.
func (r ReasonCode) Valid() bool {
	switch r {
	case ReasonSupportTicket, ReasonFraudReview, ReasonKYCReverification, ReasonLegalHold:
		return true
	default:
		return false
	}
}

// Authorizer is the object-level policy hook (handoff-03-auth.md v3):
// "a function authorize(sub, scope, target) -> allow|deny, called after
// scope validation, before the handler executes the query/retrieve."
// caller_referral and the caller-role table live in the token-issuing
// authorization server's own datastore (v3's F8 resolution) — not
// 05-data-ops's schema, and not built by this story. This interface is
// the seam: real implementation is a follow-up story against that
// datastore; this package's tests use a hand-written fake per
// decisions/test-double-strategy.md.
type Authorizer interface {
	// Authorize reports whether sub may act under scope against target
	// (a record id for ScopeReadOwn/ScopeReadAny; ignored for
	// ScopeSearch, which has no single target). A false, nil return is
	// an ordinary policy denial, not an error — errors are reserved for
	// the datastore itself being unreachable.
	Authorize(ctx context.Context, sub string, scope Scope, target string) (bool, error)
}
