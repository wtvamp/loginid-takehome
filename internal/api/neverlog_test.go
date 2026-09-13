package api

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	"loginid-takehome/internal/dao"
	"loginid-takehome/internal/model"
)

// captureLog redirects the standard library's package-level logger
// (what every real log.Printf call site in this package writes
// through) to a buffer for the duration of the test, restoring the
// previous output on cleanup. This is LT-49's application-layer
// complement to 04's static CI lint check (handoff-04-secrets.md's
// never-log list): a lint rule can catch an obviously-named variable
// passed straight to a log call, but can't catch a value that reaches a
// log line through an intermediate variable, a struct field, or a
// derived string the rule's own pattern doesn't match — this captures
// and inspects the ACTUAL bytes written during a real request, driven
// through the real handler/audit-logging code path, not a static scan
// of the source text.
//
// Not compatible with t.Parallel(): log.SetOutput/log.Writer() is
// process-wide, package-level state — no test in this package (or any
// other package, since the standard logger is shared process-wide)
// should call t.Parallel() while a test using captureLog is running, or
// its captured output will interleave with concurrent tests' own log
// lines (Oren Castellan's review, PR #59). No test in this package
// currently does; keep it that way for tests using this helper.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// TestNeverLog_TokenHandler_ClientSecretNeverLogged drives the real
// opportunistic-rehash-persist-failure path (the log call site closest
// to ever touching a secret value, tokenhandler.go's own
// "storing failed" branch) with a distinctive, never-otherwise-used
// client secret, and asserts the raw secret string never appears
// anywhere in the captured log output — item 6 of
// handoff-04-secrets.md's never-log list (API caller client_secrets, in
// plaintext or hashed).
func TestNeverLog_TokenHandler_ClientSecretNeverLogged(t *testing.T) {
	const secret = "this fixture client secret must never appear in logs"
	salt := []byte("0123456789abcdef")
	const oldTime = 2 // deliberately drifted from this package's current argon2Time
	oldHash := argon2.IDKey([]byte(secret), salt, oldTime, argon2Memory, argon2Threads, argon2KeyLen)
	driftedEncoded := encodePHC(argon2Memory, oldTime, argon2Threads, salt, oldHash)

	store := &fakeClientStore{
		records: map[string]ClientRecord{
			"drifted-client": {ClientID: "drifted-client", Name: "Drifted", ClientSecretHash: driftedEncoded, GrantedScope: Scope("profile:search"), Audience: "aud", Active: true},
		},
		// Forces the rehash PERSIST to fail, exercising the log line at
		// tokenhandler.go's "storing failed" branch — the one call site
		// closest to a temptation to log the secret it just verified.
		updateHashErr: fmt.Errorf("simulated issuer database write failure"),
	}
	limiter := &fakeGrantLimiter{allow: true}
	h := NewTokenHandler(store, limiter, testSigningKey(t), "https://issuer.test")

	buf := captureLog(t)

	form := url.Values{"grant_type": {"client_credentials"}}
	resp := postToken(t, h, form, "drifted-client", secret, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (grant already earned before the rehash attempt)", resp.StatusCode)
	}

	if strings.Contains(buf.String(), secret) {
		t.Errorf("captured log output contains the raw client secret — never-log-list item 6 violated:\n%s", buf.String())
	}
	if buf.Len() == 0 {
		t.Fatalf("captured log output is empty — the rehash-failure log line never fired, this test isn't exercising the intended path")
	}
}

// TestNeverLog_SearchAndGet_PIINeverLogged drives the real
// search-then-get flow (through the REAL StdoutAuditLogger, not a test
// fake) with seeded, distinctive PII values, and asserts none of them
// ever appear in captured log output — item 3 of the never-log list
// (PII field values; only the fact of access — sub, scope, opaque
// record id, timestamp — may be logged).
func TestNeverLog_SearchAndGet_PIINeverLogged(t *testing.T) {
	const seededName = "Zzyxwvut NeverLogThisName"
	const seededPhone = "+15559998888"
	const seededStreet = "742 NeverLog Evergreen Terrace"

	profile := model.UserProfile{
		ID: "11111111-1111-1111-1111-111111111111", Name: seededName, Phone: strp(seededPhone),
		StreetAddress: strp(seededStreet),
	}

	repo := &fakeRepository{}
	repo.profiles.searchFn = func(_ context.Context, _ dao.ProfileQuery) ([]model.UserProfile, int, error) {
		return []model.UserProfile{profile}, 1, nil
	}
	repo.profiles.getFn = func(_ context.Context, id string) (*model.UserProfile, error) {
		return &profile, nil
	}

	deps := Deps{
		Repo:         repo,
		Authz:        &fakeAuthorizer{allow: true},
		RateLimiter:  &fakeRateLimiter{allow: true},
		TouchCounter: &fakeTouchCounter{reserveOK: true},
		AuditLog:     StdoutAuditLogger{}, // the real implementation, not a test fake
	}

	buf := captureLog(t)

	searchReq := httptest.NewRequest(http.MethodPost, "/profiles/search", strings.NewReader(`{"name":"Zzyxwvut"}`))
	searchReq = searchReq.WithContext(withScope("client1", ScopeSearch))
	searchW := httptest.NewRecorder()
	NewSearchHandler(deps)(searchW, searchReq)
	if searchW.Code != http.StatusOK {
		t.Fatalf("search status = %d, want 200, body=%s", searchW.Code, searchW.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/profiles/"+profile.ID+"?reason_code=support_ticket", nil)
	getReq.SetPathValue("id", profile.ID)
	getReq = getReq.WithContext(withScope("client1", ScopeReadAny))
	getW := httptest.NewRecorder()
	NewGetProfileHandler(deps)(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200, body=%s", getW.Code, getW.Body.String())
	}

	out := buf.String()
	for _, seeded := range []string{seededName, seededPhone, seededStreet} {
		if strings.Contains(out, seeded) {
			t.Errorf("captured log output contains seeded PII value %q — never-log-list item 3 violated:\n%s", seeded, out)
		}
	}
	if out == "" {
		t.Fatalf("captured log output is empty — the audit log never fired, this test isn't exercising the intended path")
	}
}
