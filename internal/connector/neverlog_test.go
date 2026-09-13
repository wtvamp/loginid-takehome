package connector

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"loginid-takehome/internal/api"
)

// captureLog redirects the standard library's package-level logger to a
// buffer for the duration of the test — LT-49's application-layer
// complement to 04's static CI lint check (handoff-04-secrets.md's
// never-log list): this inspects the actual bytes written during a
// real request through the real handler/audit-logging path, catching
// what a static grep-style rule can miss.
//
// Not compatible with t.Parallel(): log.SetOutput/log.Writer() is
// process-wide, package-level state — no test in this package should
// call t.Parallel() while a test using captureLog is running, or its
// captured output will interleave with concurrent tests' own log lines
// (Oren Castellan's review, PR #59). No test here currently does; keep
// it that way for tests using this helper.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// TestNeverLog_AuthHandler_VendorCredentialNeverLogged drives a real
// /auth request — success AND failure branches — with a distinctive
// vendor username/password, and asserts neither ever appears in
// captured log output. Item 1 (end-user vendor password) and the
// Authorization-header half of item 2 of handoff-04-secrets.md's
// never-log list.
func TestNeverLog_AuthHandler_VendorCredentialNeverLogged(t *testing.T) {
	const username = "this fixture username must never appear in logs"
	const password = "this fixture password must never appear in logs"

	limiter := api.NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)

	cases := []struct {
		name   string
		authFn func(ctx context.Context, u, p string) (string, error)
	}{
		{name: "success", authFn: func(_ context.Context, u, p string) (string, error) {
			return "vendor-token-abc", nil
		}},
		{name: "vendor rejects", authFn: func(_ context.Context, u, p string) (string, error) {
			return "", ErrVendorAuthFailed
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vendor := &fakeVendorClient{authFn: c.authFn}
			h := NewAuthHandler(Deps{Vendor: vendor, Limiter: limiter, AuditLog: api.StdoutAuditLogger{}})

			buf := captureLog(t)

			body := strings.NewReader(`{"username":"` + username + `","password":"` + password + `"}`)
			req := withAuthContext("caller-1")(httptest.NewRequest(http.MethodPost, "/auth", body))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			out := buf.String()
			if strings.Contains(out, password) {
				t.Errorf("captured log output contains the raw vendor password — never-log-list item 1 violated:\n%s", out)
			}
			if strings.Contains(out, username) {
				t.Errorf("captured log output contains the raw vendor username — never-log-list item 11 violated:\n%s", out)
			}
		})
	}
}

// TestNeverLog_IdentityHandler_VendorAccessTokenNeverLogged drives a
// real /identity request through all three of its outcomes with a
// distinctive vendor access token, and asserts it never appears in
// captured log output — item 2 of the never-log list (vendor
// access_token, and any Authorization header value in full).
func TestNeverLog_IdentityHandler_VendorAccessTokenNeverLogged(t *testing.T) {
	const vendorToken = "this fixture vendor token must never appear in logs"

	limiter := api.NewInProcessGrantLimiter(time.Second, time.Minute, 5, nil)

	cases := []struct {
		name         string
		identityFn   func(ctx context.Context, accessToken, phone, name string) (Identity, error)
		sendNoHeader bool
	}{
		{name: "success", identityFn: func(_ context.Context, accessToken, _, _ string) (Identity, error) {
			return Identity{Name: "Jane Doe"}, nil
		}},
		{name: "vendor rejects token", identityFn: func(_ context.Context, _, _, _ string) (Identity, error) {
			return Identity{}, ErrVendorIdentityNotFound
		}},
		{name: "no token sent", sendNoHeader: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vendor := &fakeVendorClient{identityFn: c.identityFn}
			h := NewIdentityHandler(Deps{Vendor: vendor, Limiter: limiter, AuditLog: api.StdoutAuditLogger{}})

			buf := captureLog(t)

			body := strings.NewReader(`{"phone":"+15551234567","name":"Jane Doe"}`)
			req := withAuthContext("caller-1")(httptest.NewRequest(http.MethodPost, "/identity", body))
			if !c.sendNoHeader {
				req.Header.Set(vendorAccessTokenHeader, vendorToken)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			out := buf.String()
			if strings.Contains(out, vendorToken) {
				t.Errorf("captured log output contains the raw vendor access token — never-log-list item 2 violated:\n%s", out)
			}
		})
	}
}
