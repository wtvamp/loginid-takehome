package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"loginid-takehome/internal/dao"
)

// Deps bundles a handler's collaborators — the DAO, the object-level
// policy hook, the two independent limiters, and the audit sink — so
// NewSearchHandler/NewGetProfileHandler take one argument instead of
// five, and a test fake for any one of them doesn't require constructing
// the other four by hand each time.
type Deps struct {
	Repo         dao.Repository
	Authz        Authorizer
	RateLimiter  RateLimiter
	TouchCounter TouchCounter
	AuditLog     AuditLogger
}

const (
	defaultPageSize = 20
	maxPageSize     = 50
	readRateLimit   = 60 // req/min, read:own and read:any
	searchRateLimit = 10 // req/min, profile:search

	// TouchCapPerWindow and TouchCapWindow are the cumulative
	// distinct-record-touch cap's real values — ruled by Marcus Ilori
	// (02) since handoff-03-auth.md names the mechanism ("a rolling 24h
	// window") but not a number: 2,000 distinct records per caller per
	// rolling 24h, same figure for both profile:search and
	// profile:read:any. Explicitly tunable, not protocol-fixed — see
	// handoff-03-auth.md's v5 addendum for the reasoning.
	TouchCapPerWindow = 2000
	TouchCapWindow    = 24 * time.Hour
)

type searchRequest struct {
	Name     *string `json:"name,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	Region   *string `json:"region,omitempty"`
	Country  *string `json:"country,omitempty"`
	Cursor   string  `json:"cursor,omitempty"`
	PageSize int     `json:"page_size,omitempty"`
}

type searchResponse struct {
	Results    []SearchResultProfile `json:"results"`
	Total      int                   `json:"total"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

// auditCtx is the small bundle handler code passes to Deps.audit — a
// context.Context plus the sub/scope an audit event needs, since those
// two are read from AuthContext once per request rather than threaded as
// separate parameters to every call. sub/scope are zero-valued for the
// one case that precedes reading AuthContext at all (a missing/invalid
// token).
type auditCtx struct {
	ctx   context.Context
	sub   string
	scope Scope
}

func (d Deps) audit(ac auditCtx, kind AuditEventKind, recordIDs []string, reasonCode ReasonCode) {
	d.AuditLog.Log(ac.ctx, AuditEvent{
		Sub:        ac.sub,
		Scope:      ac.scope,
		Kind:       kind,
		RecordIDs:  recordIDs,
		ReasonCode: reasonCode,
		Timestamp:  time.Now().UTC(),
	})
}

// NewSearchHandler implements profile:search (handoff-03-auth.md v3).
// Order of checks, deliberately, and matching NewGetProfileHandler's
// order below (shape validation before either limiter, both limiters
// before authorize()) rather than the two handlers disagreeing on it for
// no stated reason (Oren Castellan, PR #26 review): auth context present
// -> scope is ScopeSearch -> request shape (unknown fields, empty body,
// page_size cap) -> per-minute rate limit -> cumulative touch cap ->
// authorize() -> the DAO call -> settle touches -> masked response. Scope
// mismatch and an authorize() denial return an identical client-visible
// body (F7); the audit log below is the only place that distinction is
// allowed to exist (Ingrid Solano, PR #26 review — this story's
// acceptance criteria require the distinction to be logged, not merely
// kept out of the response).
func NewSearchHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ac, ok := AuthContextFromContext(ctx)
		if !ok {
			d.audit(auditCtx{ctx: ctx}, AuditAuthnFailure, nil, "")
			writeUnauthorized(w)
			return
		}
		actx := auditCtx{ctx: ctx, sub: ac.Sub, scope: ac.Scope}
		if ac.Scope != ScopeSearch {
			d.audit(actx, AuditScopeFailure, nil, "")
			writeForbidden(w)
			return
		}

		var req searchRequest
		if r.Body != nil {
			dec := json.NewDecoder(r.Body)
			// Reject any field this shape doesn't name — in particular
			// an "offset" key, which would otherwise be silently
			// ignored rather than rejected. The API-facing contract is
			// cursor-only pagination (handoff-03-auth.md v3's F-pag);
			// a caller-supplied offset-shaped parameter is exactly the
			// bypass that contract exists to close, so it must fail
			// loudly, not be dropped quietly (Ingrid Solano, PR #26
			// review).
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				writeBadRequest(w, "malformed request body or unrecognized field")
				return
			}
		}

		// A search request with no filter field at all is rejected here
		// with a 400 before it ever reaches the DAO — the DAO's own
		// all-nil ProfileQuery is legal and returns every profile, which
		// this API must not expose directly (handoff-03-auth.md v3, F10).
		if req.Name == nil && req.Phone == nil && req.Region == nil && req.Country == nil {
			writeBadRequest(w, "at least one search filter is required")
			return
		}

		offset, err := decodeCursor(req.Cursor)
		if err != nil {
			writeBadRequest(w, "malformed cursor")
			return
		}

		pageSize := req.PageSize
		if pageSize <= 0 {
			pageSize = defaultPageSize
		}
		if pageSize > maxPageSize {
			// A bare/unconstrained request for more than this API's
			// result cap is rejected outright, not truncated
			// (handoff-03-auth.md v3) — checked here, before the query
			// is built, mirroring the same rule for authorize() below.
			writeBadRequest(w, "page_size exceeds the maximum of 50")
			return
		}

		allow, err := d.RateLimiter.Allow(ctx, ac.Sub, ScopeSearch, searchRateLimit)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if !allow {
			d.audit(actx, AuditRateLimited, nil, "")
			writeRateLimited(w)
			return
		}

		// Reserve, not just check: this books pageSize's worth of budget
		// against the window immediately, atomically with the capacity
		// check, so a second concurrent request from the same sub sees
		// this reservation and is correctly denied before either
		// request's DAO call returns — closing a check-then-act race a
		// separate Allowed()-then-RecordTouches() pair had, which could
		// let N concurrent requests each add a full page's worth of
		// touches before any of them recorded (Nolan Reyes, PR #26
		// review). The deferred release below guarantees this is settled
		// exactly once on every exit path — including one added later
		// without an explicit Settle call, and a panic unwind — rather
		// than relying on a manual Settle call at each return (Nolan
		// Reyes, second-round review).
		tr, reserved, err := reserveTouches(ctx, d.TouchCounter, ac.Sub, pageSize)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if !reserved {
			d.audit(actx, AuditCapExceeded, nil, "")
			writeTouchCapExceeded(w)
			return
		}
		defer tr.release(ctx)

		// The object-level policy hook, per v3: called after scope
		// validation (above) and request-shape validation (above), before
		// the DAO call. ScopeSearch has no single target id.
		allowed, err := d.Authz.Authorize(ctx, ac.Sub, ac.Scope, "")
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if !allowed {
			d.audit(actx, AuditPolicyDenial, nil, "")
			writeForbidden(w)
			return
		}

		q := dao.ProfileQuery{
			Name:    req.Name,
			Phone:   req.Phone,
			Region:  req.Region,
			Country: req.Country,
			Limit:   pageSize,
			Offset:  offset,
		}
		results, total, err := d.Repo.Profiles().Search(ctx, q)
		if err != nil {
			d.audit(actx, AuditRequestFailed, nil, "")
			writeDAOError(w, err)
			return
		}

		ids := make([]string, len(results))
		masked := make([]SearchResultProfile, len(results))
		for i, p := range results {
			ids[i] = p.ID
			masked[i] = toSearchResult(p)
		}
		if err := tr.settle(ctx, ids); err != nil {
			writeInternalError(w, err)
			return
		}

		d.audit(actx, AuditSuccess, ids, "")
		resp := searchResponse{Results: masked, Total: total}
		if offset+len(results) < total {
			resp.NextCursor = encodeCursor(offset + len(results))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// NewGetProfileHandler implements profile:read:own and profile:read:any
// against the same handler — the two scopes differ in what Authorize()
// checks (referral membership vs. a logged reason_code) and in whether
// the cumulative touch cap applies at all. id is read from the request
// path via r.PathValue("id") (Go 1.22+ ServeMux pattern, e.g.
// "GET /profiles/{id}"); this handler is not yet registered on any real
// route (refinement/LT-39.md's Acceptance outcome — deferred until S7's
// auth middleware exists to protect it), so tests call it directly with
// http.Request.SetPathValue.
func NewGetProfileHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ac, ok := AuthContextFromContext(ctx)
		if !ok {
			d.audit(auditCtx{ctx: ctx}, AuditAuthnFailure, nil, "")
			writeUnauthorized(w)
			return
		}
		actx := auditCtx{ctx: ctx, sub: ac.Sub, scope: ac.Scope}
		if ac.Scope != ScopeReadOwn && ac.Scope != ScopeReadAny {
			d.audit(actx, AuditScopeFailure, nil, "")
			writeForbidden(w)
			return
		}

		id := r.PathValue("id")
		if strings.TrimSpace(id) == "" {
			writeBadRequest(w, "id is required")
			return
		}

		// reasonCode is validated here and logged with every audit event
		// on this request (handoff-03-auth.md v3: "reason_code, when the
		// scope is profile:read:any" is a required audit-log field).
		// There is deliberately no other consumer of the value — no
		// policy decision in this story reads it beyond validating its
		// membership in the closed enum; threading it into Authorizer's
		// signature is a follow-up story's job if a real policy
		// implementation ever needs it for more than logging (Oren
		// Castellan, PR #26 review).
		var reasonCode ReasonCode
		if ac.Scope == ScopeReadAny {
			reasonCode = ReasonCode(r.URL.Query().Get("reason_code"))
			if !reasonCode.Valid() {
				writeBadRequest(w, "reason_code is required for profile:read:any and must be one of the closed enum values")
				return
			}
		}

		allow, err := d.RateLimiter.Allow(ctx, ac.Sub, ac.Scope, readRateLimit)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if !allow {
			d.audit(actx, AuditRateLimited, nil, reasonCode)
			writeRateLimited(w)
			return
		}

		// The cumulative cap applies to profile:search and
		// profile:read:any (handoff-03-auth.md v3, verbatim) — not
		// profile:read:own, which is bounded by the caller_referral
		// relationship rather than an open id space, so it isn't the
		// enumeration surface this control exists for. A valid, logged
		// reason_code on read:any satisfies Authorize() below but does
		// not exempt the caller from this cap — the exact regression v3
		// fixed (an earlier wording made this counter a no-op for
		// read:any, since that scope requires a reason_code on every call
		// by design).
		var tr *touchReservation
		if ac.Scope == ScopeReadAny {
			var reserved bool
			tr, reserved, err = reserveTouches(ctx, d.TouchCounter, ac.Sub, 1)
			if err != nil {
				writeInternalError(w, err)
				return
			}
			if !reserved {
				d.audit(actx, AuditCapExceeded, nil, reasonCode)
				writeTouchCapExceeded(w)
				return
			}
			// Deferred release guarantees this is settled exactly once
			// on every exit path below, including one added later
			// without an explicit Settle call (Nolan Reyes, PR #26
			// review, second round) — tr is nil for ScopeReadOwn, and
			// (*touchReservation)(nil).release is a documented no-op.
			defer tr.release(ctx)
		}

		allowed, err := d.Authz.Authorize(ctx, ac.Sub, ac.Scope, id)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		if !allowed {
			d.audit(actx, AuditPolicyDenial, []string{id}, reasonCode)
			writeForbidden(w)
			return
		}

		p, err := d.Repo.Profiles().Get(ctx, id)
		if err != nil {
			d.audit(actx, AuditRequestFailed, []string{id}, reasonCode)
			writeDAOError(w, err)
			return
		}

		if err := tr.settle(ctx, []string{p.ID}); err != nil {
			writeInternalError(w, err)
			return
		}

		d.audit(actx, AuditSuccess, []string{p.ID}, reasonCode)
		writeJSON(w, http.StatusOK, toProfileResponse(p))
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
