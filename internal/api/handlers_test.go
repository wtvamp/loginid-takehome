package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

func withScope(sub string, scope Scope) context.Context {
	return WithAuthContext(context.Background(), AuthContext{Sub: sub, Scope: scope})
}

func strp(s string) *string { return &s }

// ---- profile:search ----

func TestSearchHandler_MissingAuthContext_401(t *testing.T) {
	h := NewSearchHandler(allowAllDeps())
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestSearchHandler_WrongScope_403(t *testing.T) {
	h := NewSearchHandler(allowAllDeps())
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	req = req.WithContext(withScope("client1", ScopeReadOwn))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// TestSearchHandler_EmptyBody_400 is LT-39's acceptance criterion: "a
// search request with no filter field at all is rejected by this handler
// with a 400 before it reaches the DAO."
func TestSearchHandler_EmptyBody_400(t *testing.T) {
	d := allowAllDeps()
	repo := d.Repo.(*fakeRepository)
	repo.profiles.searchFn = func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
		t.Fatal("Search must not be called for an empty request body")
		return nil, 0, nil
	}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestSearchHandler_PageSizeAboveCap_RejectedNotTruncated is LT-39's
// "rejected outright, not truncated" criterion, checked before the query
// is built.
func TestSearchHandler_PageSizeAboveCap_RejectedNotTruncated(t *testing.T) {
	d := allowAllDeps()
	repo := d.Repo.(*fakeRepository)
	repo.profiles.searchFn = func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
		t.Fatal("Search must not be called when page_size exceeds the cap")
		return nil, 0, nil
	}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a","page_size":51}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestSearchHandler_ResponseIsMasked asserts the search-response
// serialization path is distinct from the read-response one: the JSON
// body must carry the masked phone shape and must never carry
// street_address/postal_code/country at all.
func TestSearchHandler_ResponseIsMasked(t *testing.T) {
	d := allowAllDeps()
	repo := d.Repo.(*fakeRepository)
	phone := "+15551234567"
	street := "123 Main St"
	repo.profiles.searchFn = func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
		return []model.UserProfile{{ID: "p1", Name: "Jane Doe", Phone: &phone, StreetAddress: &street, Region: strp("CA")}}, 1, nil
	}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"jane"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	results, _ := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1 entry", results)
	}
	result := results[0].(map[string]any)
	if result["phone"] != "***-***-4567" {
		t.Errorf("phone = %v, want masked", result["phone"])
	}
	if _, ok := result["street_address"]; ok {
		t.Errorf("search response must never carry street_address, got %v", result)
	}
}

func TestSearchHandler_RateLimited_429(t *testing.T) {
	d := allowAllDeps()
	d.RateLimiter = &fakeRateLimiter{allow: false}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", w.Code)
	}
}

// TestSearchHandler_TouchCapExceeded_429NotServed is the PO review
// script's item 4a: a call made after the threshold is crossed returns
// 429 and is NOT served — not merely observed to exceed the threshold.
func TestSearchHandler_TouchCapExceeded_429NotServed(t *testing.T) {
	d := allowAllDeps()
	d.TouchCounter = &fakeTouchCounter{reserveOK: false}
	repo := d.Repo.(*fakeRepository)
	repo.profiles.searchFn = func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
		t.Fatal("Search must not be called once the touch cap is already exceeded")
		return nil, 0, nil
	}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", w.Code)
	}
}

func TestSearchHandler_AuthorizeDenied_403(t *testing.T) {
	d := allowAllDeps()
	d.Authz = &fakeAuthorizer{allow: false}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestSearchHandler_DAOError_NeverLeaksRawMessage(t *testing.T) {
	d := allowAllDeps()
	repo := d.Repo.(*fakeRepository)
	repo.profiles.searchFn = func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
		return nil, 0, errUnmapped
	}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "supersecret-driver-detail") {
		t.Errorf("response leaked the raw error: %s", w.Body.String())
	}
}

var errUnmapped = &sentinelStringer{"supersecret-driver-detail: DETAIL Key (username)=(x) already exists"}

type sentinelStringer struct{ s string }

func (e *sentinelStringer) Error() string { return e.s }

// ---- profile:read:own / profile:read:any ----

func newGetRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/profiles/"+id, nil)
	req.SetPathValue("id", id)
	return req
}

func TestGetProfileHandler_ReadOwn_Success(t *testing.T) {
	d := allowAllDeps()
	repo := d.Repo.(*fakeRepository)
	repo.profiles.getFn = func(ctx context.Context, id string) (*model.UserProfile, error) {
		return &model.UserProfile{ID: id, Name: "Jane Doe"}, nil
	}
	h := NewGetProfileHandler(d)
	req := newGetRequest("11111111-1111-1111-1111-111111111111")
	req = req.WithContext(withScope("client1", ScopeReadOwn))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestGetProfileHandler_ReadAny_RequiresValidReasonCode covers the closed
// enum requirement.
func TestGetProfileHandler_ReadAny_RequiresValidReasonCode(t *testing.T) {
	d := allowAllDeps()
	h := NewGetProfileHandler(d)
	req := newGetRequest("11111111-1111-1111-1111-111111111111")
	req = req.WithContext(withScope("client1", ScopeReadAny))
	w := httptest.NewRecorder()
	h(w, req) // no reason_code query param at all
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when reason_code is missing", w.Code)
	}

	req2 := newGetRequest("11111111-1111-1111-1111-111111111111")
	req2.URL.RawQuery = "reason_code=not_a_real_reason"
	req2 = req2.WithContext(withScope("client1", ScopeReadAny))
	w2 := httptest.NewRecorder()
	h(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unrecognized reason_code", w2.Code)
	}
}

// TestGetProfileHandler_ReadAny_ValidReasonCodeStillHitsTouchCap is the
// PO review script's item 4: the specific regression handoff-03-auth.md
// v3 fixed — a valid, logged reason_code satisfies Authorize() but does
// NOT exempt the caller from the cumulative touch cap.
func TestGetProfileHandler_ReadAny_ValidReasonCodeStillHitsTouchCap(t *testing.T) {
	d := allowAllDeps()
	d.TouchCounter = &fakeTouchCounter{reserveOK: false} // already over cap
	repo := d.Repo.(*fakeRepository)
	repo.profiles.getFn = func(ctx context.Context, id string) (*model.UserProfile, error) {
		t.Fatal("Get must not be called once the touch cap is already exceeded, even with a valid reason_code")
		return nil, nil
	}
	h := NewGetProfileHandler(d)
	req := newGetRequest("11111111-1111-1111-1111-111111111111")
	req.URL.RawQuery = "reason_code=support_ticket"
	req = req.WithContext(withScope("client1", ScopeReadAny))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429 — a valid reason_code must not exempt read:any from the cumulative cap", w.Code)
	}
}

// TestGetProfileHandler_ScopeFailureAndAuthorizeDenial_IdenticalBody is
// F7 (decisions/error-semantics.md): a scope failure and an authorize()
// policy denial return an identical client-visible body — distinguished
// ONLY in the audit log (Ingrid Solano, PR #26 review: this story's own
// acceptance criteria require that distinction to actually be logged
// somewhere, not merely kept out of the response).
func TestGetProfileHandler_ScopeFailureAndAuthorizeDenial_IdenticalBody(t *testing.T) {
	// Scope failure: caller has ScopeSearch but hits the read endpoint.
	d1 := allowAllDeps()
	audit1 := d1.AuditLog.(*fakeAuditLogger)
	h1 := NewGetProfileHandler(d1)
	req1 := newGetRequest("11111111-1111-1111-1111-111111111111")
	req1 = req1.WithContext(withScope("client1", ScopeSearch))
	w1 := httptest.NewRecorder()
	h1(w1, req1)

	// authorize() denial: right scope, policy says no.
	d2 := allowAllDeps()
	d2.Authz = &fakeAuthorizer{allow: false}
	audit2 := d2.AuditLog.(*fakeAuditLogger)
	h2 := NewGetProfileHandler(d2)
	req2 := newGetRequest("11111111-1111-1111-1111-111111111111")
	req2 = req2.WithContext(withScope("client1", ScopeReadOwn))
	w2 := httptest.NewRecorder()
	h2(w2, req2)

	if w1.Code != http.StatusForbidden || w2.Code != http.StatusForbidden {
		t.Fatalf("both must be 403, got %d and %d", w1.Code, w2.Code)
	}
	if w1.Body.String() != w2.Body.String() {
		t.Errorf("scope failure body %q must be identical to authorize() denial body %q", w1.Body.String(), w2.Body.String())
	}

	if len(audit1.events) != 1 || audit1.events[0].Kind != AuditScopeFailure {
		t.Errorf("scope-failure path audit events = %+v, want exactly one AuditScopeFailure", audit1.events)
	}
	if len(audit2.events) != 1 || audit2.events[0].Kind != AuditPolicyDenial {
		t.Errorf("authorize()-denial path audit events = %+v, want exactly one AuditPolicyDenial", audit2.events)
	}
}

// TestGetProfileHandler_ReadOwn_DoesNotHitTouchCap: the cumulative
// distinct-record-touch cap applies to profile:search and
// profile:read:any (handoff-03-auth.md v3, verbatim) — profile:read:own
// is bounded by the caller_referral relationship, not an open id space,
// so it must not be gated by TouchCounter at all (Oren Castellan, PR #26
// review: an earlier version of this handler applied the cap to
// read:own too, contradicting the package's own doc comment, and nothing
// caught it).
func TestGetProfileHandler_ReadOwn_DoesNotHitTouchCap(t *testing.T) {
	d := allowAllDeps()
	// A TouchCounter that denies everything — if read:own ever calls it,
	// the request fails; it must not be called at all for this scope.
	d.TouchCounter = &fakeTouchCounter{reserveOK: false}
	repo := d.Repo.(*fakeRepository)
	repo.profiles.getFn = func(ctx context.Context, id string) (*model.UserProfile, error) {
		return &model.UserProfile{ID: id, Name: "Jane Doe"}, nil
	}
	h := NewGetProfileHandler(d)
	req := newGetRequest("11111111-1111-1111-1111-111111111111")
	req = req.WithContext(withScope("client1", ScopeReadOwn))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 — profile:read:own must not be gated by the cumulative touch cap", w.Code)
	}
}

// TestSearchHandler_RejectsUnknownField is Ingrid Solano's PR #26 finding:
// an unrecognized field (in particular an "offset" key, the exact bypass
// the opaque-cursor contract exists to close) must be rejected outright,
// not silently ignored.
func TestSearchHandler_RejectsUnknownField(t *testing.T) {
	h := NewSearchHandler(allowAllDeps())
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a","offset":40}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unrecognized \"offset\" field", w.Code)
	}
}

// TestSearchHandler_SuccessAuditsWithRecordIDs asserts a successful
// search still emits an audit event (the field list requires "record
// id(s) touched" and "request outcome" on every call, not only denials).
func TestSearchHandler_SuccessAuditsWithRecordIDs(t *testing.T) {
	d := allowAllDeps()
	audit := d.AuditLog.(*fakeAuditLogger)
	repo := d.Repo.(*fakeRepository)
	repo.profiles.searchFn = func(ctx context.Context, q dao.ProfileQuery) ([]model.UserProfile, int, error) {
		return []model.UserProfile{{ID: "p1", Name: "Jane"}}, 1, nil
	}
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"jane"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(audit.events) != 1 || audit.events[0].Kind != AuditSuccess {
		t.Fatalf("audit events = %+v, want exactly one AuditSuccess", audit.events)
	}
	if len(audit.events[0].RecordIDs) != 1 || audit.events[0].RecordIDs[0] != "p1" {
		t.Errorf("audit event RecordIDs = %v, want [p1]", audit.events[0].RecordIDs)
	}
}

// TestSearchHandler_DeniedRequestReleasesTouchReservation asserts that
// when authorize() denies a request after Reserve already booked its
// budget, that budget is released via Settle rather than permanently
// consumed — a denied request must not itself count against the cap.
func TestSearchHandler_DeniedRequestReleasesTouchReservation(t *testing.T) {
	d := allowAllDeps()
	d.Authz = &fakeAuthorizer{allow: false}
	tc := d.TouchCounter.(*fakeTouchCounter)
	h := NewSearchHandler(d)
	req := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"a"}`))
	req = req.WithContext(withScope("client1", ScopeSearch))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if len(tc.settledReserved) != 1 || tc.settledReserved[0] != defaultPageSize {
		t.Errorf("Settle calls = %v, want exactly one release of the full reserved page size (%d)", tc.settledReserved, defaultPageSize)
	}
	if len(tc.settledIDs) != 0 {
		t.Errorf("a denied request must settle with no record ids, got %v", tc.settledIDs)
	}
}

func TestGetProfileHandler_MissingID_400(t *testing.T) {
	d := allowAllDeps()
	h := NewGetProfileHandler(d)
	req := newGetRequest("")
	req = req.WithContext(withScope("client1", ScopeReadOwn))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetProfileHandler_NotFound(t *testing.T) {
	d := allowAllDeps()
	repo := d.Repo.(*fakeRepository)
	repo.profiles.getFn = func(ctx context.Context, id string) (*model.UserProfile, error) {
		return nil, dao.ErrNotFound
	}
	h := NewGetProfileHandler(d)
	req := newGetRequest("11111111-1111-1111-1111-111111111111")
	req = req.WithContext(withScope("client1", ScopeReadOwn))
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
