package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"loginid-takehome/internal/api"
)

// fakeVendorClient implements VendorClient with configurable funcs, per
// decisions/test-double-strategy.md.
type fakeVendorClient struct {
	authFn     func(ctx context.Context, username, password string) (string, error)
	identityFn func(ctx context.Context, accessToken, phone, name string) (Identity, error)
}

func (f *fakeVendorClient) Authenticate(ctx context.Context, username, password string) (string, error) {
	return f.authFn(ctx, username, password)
}
func (f *fakeVendorClient) FetchIdentity(ctx context.Context, accessToken, phone, name string) (Identity, error) {
	return f.identityFn(ctx, accessToken, phone, name)
}

func withAuthContext(sub string) func(*http.Request) *http.Request {
	return func(r *http.Request) *http.Request {
		return r.WithContext(api.WithAuthContext(r.Context(), api.AuthContext{Sub: sub, Scope: api.ScopeConnectorIdentityLookup}))
	}
}

func TestNewAuthHandler_Success(t *testing.T) {
	vendor := &fakeVendorClient{
		authFn: func(_ context.Context, username, password string) (string, error) {
			if username == "u" && password == "p" {
				return "vendor-token-abc", nil
			}
			return "", ErrVendorAuthFailed
		},
	}
	limiter := api.NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)
	h := NewAuthHandler(Deps{Vendor: vendor, Limiter: limiter, AuditLog: api.StdoutAuditLogger{}})

	body := strings.NewReader(`{"username":"u","password":"p"}`)
	req := withAuthContext("caller-1")(httptest.NewRequest(http.MethodPost, "/auth", body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var resp authResponseBody
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.AccessToken != "vendor-token-abc" {
		t.Errorf("access_token = %q, want vendor-token-abc", resp.AccessToken)
	}
}

func TestNewAuthHandler_VendorRejects_GenericFailure(t *testing.T) {
	vendor := &fakeVendorClient{
		authFn: func(context.Context, string, string) (string, error) { return "", ErrVendorAuthFailed },
	}
	limiter := api.NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)
	h := NewAuthHandler(Deps{Vendor: vendor, Limiter: limiter, AuditLog: api.StdoutAuditLogger{}})

	req := withAuthContext("caller-1")(httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"username":"u","password":"wrong"}`)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var body identityErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body.Error != "vendor_authentication_failed" {
		t.Errorf("error = %q — must not distinguish unknown-username from wrong-password (connector-security.md §4)", body.Error)
	}
}

// TestNewIdentityHandler_ThreeFailureBranches_ContentIdenticalAndTimingClose
// is the enforcement site for both F3 (content) and Marcus Ilori's
// ~100ms floor ruling (timing) — refinement/LT-41.md's own named test
// shape: no-token, bad-token, and vendor-rejected must produce
// byte-identical response bodies/status, and must return within a
// bounded time delta of each other, not merely "both fast" or "both
// slow" independently.
func TestNewIdentityHandler_ThreeFailureBranches_ContentIdenticalAndTimingClose(t *testing.T) {
	// "bad token" — a local, near-instant rejection.
	badTokenVendor := &fakeVendorClient{
		identityFn: func(context.Context, string, string, string) (Identity, error) {
			return Identity{}, ErrVendorIdentityNotFound
		},
	}
	// "vendor rejected it" — simulates an actual network round-trip's
	// latency (well under the floor, so the floor is what equalizes it).
	vendorRejectedVendor := &fakeVendorClient{
		identityFn: func(context.Context, string, string, string) (Identity, error) {
			time.Sleep(15 * time.Millisecond)
			return Identity{}, ErrVendorIdentityNotFound
		},
	}

	// Each call gets its own fresh limiter — this test is about content/
	// timing equivalence across failure branches, not rate-limiting
	// interaction, so a shared limiter's backoff from an earlier call in
	// this same test must not affect a later one.
	runCase := func(vendor VendorClient, sendToken bool) (int, []byte, time.Duration) {
		limiter := api.NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)
		h := NewIdentityHandler(Deps{Vendor: vendor, Limiter: limiter, AuditLog: api.StdoutAuditLogger{}})
		req := withAuthContext("caller-1")(httptest.NewRequest(http.MethodPost, "/identity", strings.NewReader(`{"phone":"+15555550100","name":"Jane"}`)))
		if sendToken {
			req.Header.Set(vendorAccessTokenHeader, "some-token")
		}
		w := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(w, req)
		elapsed := time.Since(start)
		return w.Code, w.Body.Bytes(), elapsed
	}

	noTokenStatus, noTokenBody, noTokenElapsed := runCase(badTokenVendor, false)
	badTokenStatus, badTokenBody, badTokenElapsed := runCase(badTokenVendor, true)
	vendorRejectedStatus, vendorRejectedBody, vendorRejectedElapsed := runCase(vendorRejectedVendor, true)

	if noTokenStatus != http.StatusUnauthorized || badTokenStatus != http.StatusUnauthorized || vendorRejectedStatus != http.StatusUnauthorized {
		t.Fatalf("statuses = %d, %d, %d, want all 401", noTokenStatus, badTokenStatus, vendorRejectedStatus)
	}
	if string(noTokenBody) != string(badTokenBody) || string(badTokenBody) != string(vendorRejectedBody) {
		t.Errorf("response bodies differ: no-token=%q bad-token=%q vendor-rejected=%q — must be byte-identical (connector-security.md §1/F3)",
			noTokenBody, badTokenBody, vendorRejectedBody)
	}

	// All three must land close to the ~100ms floor (identityFailureFloor),
	// not just "all fast" or "all slow" independently — the floor pads
	// the fast paths UP, it doesn't cap the slow path down, so every
	// branch should be at least the floor and not wildly divergent from
	// it (a generous upper bound accounts for test-environment jitter).
	for name, elapsed := range map[string]time.Duration{
		"no-token":        noTokenElapsed,
		"bad-token":       badTokenElapsed,
		"vendor-rejected": vendorRejectedElapsed,
	} {
		if elapsed < identityFailureFloor {
			t.Errorf("%s branch returned in %v, want at least the %v floor", name, elapsed, identityFailureFloor)
		}
		if elapsed > identityFailureFloor+50*time.Millisecond {
			t.Errorf("%s branch returned in %v, want within ~50ms of the %v floor", name, elapsed, identityFailureFloor)
		}
	}
}

func TestNewIdentityHandler_Success_NoFloorApplied(t *testing.T) {
	limiter := api.NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)
	vendor := &fakeVendorClient{
		identityFn: func(context.Context, string, string, string) (Identity, error) {
			return Identity{Name: "Jane Demo", Phone: "+15555550100", Country: "US"}, nil
		},
	}
	h := NewIdentityHandler(Deps{Vendor: vendor, Limiter: limiter, AuditLog: api.StdoutAuditLogger{}})

	req := withAuthContext("caller-1")(httptest.NewRequest(http.MethodPost, "/identity", strings.NewReader(`{"phone":"+15555550100","name":"Jane"}`)))
	req.Header.Set(vendorAccessTokenHeader, "good-token")
	w := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if elapsed >= identityFailureFloor {
		t.Errorf("success path took %v — the failure floor must not apply to a success response", elapsed)
	}
	var body identityResponseBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Name != "Jane Demo" || body.Country != "US" {
		t.Errorf("unexpected identity in response: %+v", body)
	}
}

// TestVendorCallLimiter_FailureRatioTripsBeforeVolume is LT-41's own
// named enforcement site for attack-tree leaf 5: a caller with a high
// FAILURE ratio must be blocked before a caller with high VOLUME but
// mostly successful calls ever would be. Reuses api.GrantLimiter (PR
// #30/#34) directly, per this story's non-goal ("no shared rate-limit
// store... this story's own in-process rate limiting is sufficient,
// matching the pattern already used for RateLimiter/TouchCounter/
// GrantLimiter") — GrantLimiter's backoff is driven purely by
// CONSECUTIVE failures (RecordSuccess resets it to zero), so a caller
// who never has two failures in a row can generate unlimited volume
// without ever tripping the limiter, while a caller failing repeatedly
// trips within a handful of attempts.
func TestVendorCallLimiter_FailureRatioTripsBeforeVolume(t *testing.T) {
	limiter := api.NewInProcessGrantLimiter(10*time.Millisecond, time.Hour, 100, nil)
	ctx := context.Background()

	// High-failure-ratio caller: fails every attempt in a row.
	blockedAfter := -1
	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow(ctx, "attacker", "10.0.0.1")
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !allowed {
			blockedAfter = i
			break
		}
		limiter.RecordFailure(ctx, "attacker", "10.0.0.1")
	}
	if blockedAfter == -1 {
		t.Fatalf("high-failure-ratio caller was never blocked across 10 consecutive failures")
	}

	// High-volume-but-mostly-successful caller: never blocked, no matter
	// how many attempts, because every attempt succeeds (no consecutive
	// failures ever accumulate).
	for i := 0; i < 1000; i++ {
		allowed, err := limiter.Allow(ctx, "legit-client", "10.0.0.2")
		if err != nil {
			t.Fatalf("Allow: %v", err)
		}
		if !allowed {
			t.Fatalf("high-volume-but-successful caller was blocked after %d attempts, want never (volume alone must not trip this limiter)", i)
		}
		limiter.RecordSuccess(ctx, "legit-client", "10.0.0.2")
	}

	if blockedAfter >= 1000 {
		t.Fatalf("failure-ratio caller (blocked at attempt %d) did not trip before the 1000-attempt volume caller (never tripped) — want the opposite", blockedAfter)
	}
}
