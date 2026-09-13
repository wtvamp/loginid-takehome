package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"loginid-takehome/internal/dao"
)

// Deps bundles a handler's collaborators — the DAO, the object-level
// policy hook, and the two independent limiters — so NewSearchHandler/
// NewGetProfileHandler take one argument instead of four, and a test
// fake for any one of them doesn't require constructing the other three
// by hand each time.
type Deps struct {
	Repo         dao.Repository
	Authz        Authorizer
	RateLimiter  RateLimiter
	TouchCounter TouchCounter
}

const (
	defaultPageSize = 20
	maxPageSize     = 50
	readRateLimit   = 60 // req/min, read:own and read:any
	searchRateLimit = 10 // req/min, profile:search
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

// NewSearchHandler implements profile:search (handoff-03-auth.md v3).
// Order of checks, deliberately: auth context present -> scope is
// ScopeSearch -> per-minute rate limit -> cumulative touch cap -> request
// shape (empty body, bare-wildcard-shaped unconstrained query) ->
// authorize() -> the DAO call -> record touches -> masked response. Scope
// mismatch and an authorize() denial return an identical body (F7); only
// the audit log (not built by this story — see refinement/LT-39.md's
// non-goals) would distinguish them.
func NewSearchHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ac, ok := AuthContextFromContext(ctx)
		if !ok {
			writeUnauthorized(w)
			return
		}
		if ac.Scope != ScopeSearch {
			writeForbidden(w)
			return
		}

		allow, err := d.RateLimiter.Allow(ctx, ac.Sub, ScopeSearch, searchRateLimit)
		if err != nil {
			writeDAOError(w, err)
			return
		}
		if !allow {
			writeRateLimited(w)
			return
		}

		capOK, err := d.TouchCounter.Allowed(ctx, ac.Sub)
		if err != nil {
			writeDAOError(w, err)
			return
		}
		if !capOK {
			writeTouchCapExceeded(w)
			return
		}

		var req searchRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeBadRequest(w, "malformed request body")
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

		// The object-level policy hook, per v3: called after scope
		// validation (above) and request-shape validation (above), before
		// the DAO call. ScopeSearch has no single target id.
		allowed, err := d.Authz.Authorize(ctx, ac.Sub, ac.Scope, "")
		if err != nil {
			writeDAOError(w, err)
			return
		}
		if !allowed {
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
			writeDAOError(w, err)
			return
		}

		ids := make([]string, len(results))
		masked := make([]SearchResultProfile, len(results))
		for i, p := range results {
			ids[i] = p.ID
			masked[i] = toSearchResult(p)
		}
		if _, err := d.TouchCounter.RecordTouches(ctx, ac.Sub, ids); err != nil {
			writeDAOError(w, err)
			return
		}

		resp := searchResponse{Results: masked, Total: total}
		if offset+len(results) < total {
			resp.NextCursor = encodeCursor(offset + len(results))
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// NewGetProfileHandler implements profile:read:own and profile:read:any
// against the same handler — the two scopes differ only in what
// Authorize() checks (referral membership vs. a logged reason_code), not
// in the handler's own shape. id is read from the request path via
// r.PathValue("id") (Go 1.22+ ServeMux pattern, e.g.
// "GET /profiles/{id}"); this handler is not yet registered on any real
// route (refinement/LT-39.md's Acceptance outcome — deferred until S7's
// auth middleware exists to protect it), so tests call it directly with
// http.Request.SetPathValue.
func NewGetProfileHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ac, ok := AuthContextFromContext(ctx)
		if !ok {
			writeUnauthorized(w)
			return
		}
		if ac.Scope != ScopeReadOwn && ac.Scope != ScopeReadAny {
			writeForbidden(w)
			return
		}

		id := r.PathValue("id")
		if strings.TrimSpace(id) == "" {
			writeBadRequest(w, "id is required")
			return
		}

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
			writeDAOError(w, err)
			return
		}
		if !allow {
			writeRateLimited(w)
			return
		}

		// Applies to profile:read:any unconditionally, whether or not a
		// reason_code is present — the exact regression handoff-03-auth.md
		// v3 fixed (an earlier wording made this counter a no-op for
		// read:any, since that scope requires a reason_code on every call
		// by design; a valid, logged reason_code satisfies Authorize()
		// below but does not exempt the caller from this cap).
		capOK, err := d.TouchCounter.Allowed(ctx, ac.Sub)
		if err != nil {
			writeDAOError(w, err)
			return
		}
		if !capOK {
			writeTouchCapExceeded(w)
			return
		}

		allowed, err := d.Authz.Authorize(ctx, ac.Sub, ac.Scope, id)
		if err != nil {
			writeDAOError(w, err)
			return
		}
		if !allowed {
			writeForbidden(w)
			return
		}

		p, err := d.Repo.Profiles().Get(ctx, id)
		if err != nil {
			writeDAOError(w, err)
			return
		}

		if _, err := d.TouchCounter.RecordTouches(ctx, ac.Sub, []string{p.ID}); err != nil {
			writeDAOError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, toProfileResponse(p))
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
