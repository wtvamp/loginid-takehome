package app

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"loginid-takehome/internal/api"
	"loginid-takehome/internal/config"
)

// TestDemoHandler_ServesEmbeddedPage confirms GET /demo on the verifier
// router serves the go:embed'd page — refinement/LT-53.md's first
// acceptance criterion.
func TestDemoHandler_ServesEmbeddedPage(t *testing.T) {
	cfg := config.Config{AuthJWTIssuer: testIssuer, AuthJWTAudience: testAudience, AuthJWKSURL: "http://unused.invalid"}
	router := NewVerifierRouter(cfg, &fakeRepo{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("body doesn't look like an HTML document")
	}
	if !strings.Contains(body, "Mint token") {
		t.Errorf("body missing expected page content (credential mint control)")
	}
}

// TestDemoHandler_NoAuthRequired proves this route deliberately sits
// outside the JWT middleware chain — it's a static page, not one of the
// two protected routes — while still reachable on the exact same mux as
// everything else this router serves (no special-cased bypass to add or
// omit: it simply never had auth applied to it in the first place, the
// same as /healthz).
func TestDemoHandler_NoAuthRequired(t *testing.T) {
	cfg := config.Config{AuthJWTIssuer: testIssuer, AuthJWTAudience: testAudience, AuthJWKSURL: "http://unused.invalid"}
	router := NewVerifierRouter(cfg, &fakeRepo{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/demo", nil) // no Authorization header at all
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 even with no bearer token — /demo is a static page, not a protected route", w.Code)
	}
}

// TestDemoPage_NoSecretLiteralOrExternalURL is this story's own
// engineering acceptance criterion, stated verbatim in the PM's
// routing message: "a test asserts the embedded page has no
// client_secret literal and no external URL." Reads the SAME bytes the
// handler serves (demoPage, the go:embed'd var) directly, rather than
// re-deriving them through an HTTP round trip.
func TestDemoPage_NoSecretLiteralOrExternalURL(t *testing.T) {
	body := string(demoPage)

	// No hardcoded QA client id or secret-shaped literal baked into the
	// page's own source — the visitor supplies both at runtime.
	forbidden := []string{
		"qa-review-client", // the real, seeded QA client_id — never hardcoded here
		"client_secret=",   // a filled-in query/body literal, as opposed to the bare
		// field label/attribute name "client secret" this page's
		// own copy and input labels legitimately use
		"$argon2id$", // a PHC-format hash, the shape a real stored secret takes
	}
	for _, s := range forbidden {
		if strings.Contains(body, s) {
			t.Errorf("embedded page contains forbidden literal %q — a real credential value must never be baked into this page's source", s)
		}
	}

	// No external asset request anywhere — every network call this page
	// makes must be same-origin (relative paths only), and no external
	// font/script/stylesheet may be loaded (Warren's design brief: no
	// external assets, ever).
	externalURL := regexp.MustCompile(`https?://[^'"\s]+`)
	if m := externalURL.FindString(body); m != "" {
		t.Errorf("embedded page references an external URL (%q) — this page must be entirely self-contained, same-origin only", m)
	}
}

// TestDemoPage_SecretInputHardening is Marcus Ilori's (02) added
// criterion: a plain credential-shaped <input> can trigger a browser's
// own "save this password?" prompt regardless of what this page's JS
// does with the value, so the input itself must be hardened against
// that — type="password" (so it isn't held in the DOM as visible
// plaintext), autocomplete set away from the browser's default
// password-save heuristic, and a name/id that doesn't pattern-match
// what a password manager looks for ("password"/"secret" as the
// literal attribute value).
func TestDemoPage_SecretInputHardening(t *testing.T) {
	body := string(demoPage)

	if !strings.Contains(body, `id="qa-token-input"`) {
		t.Fatalf("expected the QA secret input's id to be qa-token-input (deliberately not \"password\"/\"secret\") — page markup may have changed without updating this test")
	}
	inputTag := regexp.MustCompile(`<input[^>]*id="qa-token-input"[^>]*>`).FindString(body)
	if inputTag == "" {
		t.Fatalf("could not find the qa-token-input <input> tag in the embedded page")
	}
	if !strings.Contains(inputTag, `type="password"`) {
		t.Errorf("qa-token-input must be type=\"password\": %s", inputTag)
	}
	if strings.Contains(inputTag, `autocomplete="on"`) || !strings.Contains(inputTag, "autocomplete=") {
		t.Errorf("qa-token-input must set autocomplete away from the browser default: %s", inputTag)
	}
	if strings.Contains(inputTag, `name="password"`) || strings.Contains(inputTag, `name="secret"`) {
		t.Errorf("qa-token-input's name must not literally be \"password\"/\"secret\" (password-manager heuristic bait): %s", inputTag)
	}
}

// TestDemoPage_SecretNeverStoredOrLoggedClientSide is a static-content
// check standing in for "read the shipped JS directly" (refinement/
// LT-53.md's PO review script step 2): confirm the page's own script
// never writes to localStorage/sessionStorage/document.cookie at all
// (the QA secret and the minted token both stay in-memory only, per
// this story's own architecture criterion) and never passes anything
// to console.log/console.debug (no accidental client-side logging of a
// value this page holds).
func TestDemoPage_SecretNeverStoredOrLoggedClientSide(t *testing.T) {
	body := string(demoPage)
	forbidden := []string{"localStorage", "sessionStorage", "document.cookie", "console.log", "console.debug"}
	for _, s := range forbidden {
		if strings.Contains(body, s) {
			t.Errorf("embedded page's script references %q — the QA secret and minted token must stay in memory only, never persisted or logged client-side", s)
		}
	}
}

// TestDemoPage_SecretOnlySentToAuthToken is Marcus Ilori's (02)
// enforcement site for the credential-handling criterion: the only
// fetch/network call in this page's script that ever touches the
// secret input's value is the one to /auth/token. This walks the
// script textually rather than executing it (no JS runtime in `go
// test`), asserting the secret-reading expression appears exactly
// once in the whole file and it's inside the mint handler that posts
// to /auth/token.
func TestDemoPage_SecretOnlySentToAuthToken(t *testing.T) {
	body := string(demoPage)
	secretRead := `el("qa-token-input").value`
	secretActuallyRead := `+ el("qa-token-input").value` // used (concatenated) to build the Basic-auth header
	secretCleared := `el("qa-token-input").value = ""`   // reset to empty — a write, not a read

	// Exactly one real read (to build the Basic-auth header for the
	// mint call) — never a second site that could mean "send it
	// somewhere else too".
	if n := strings.Count(body, secretActuallyRead); n != 1 {
		t.Errorf("secret input's value is read (used) %d times, want exactly 1 (only to build the mint call's Basic-auth header)", n)
	}
	// Cleared after both the success and the error path of the mint
	// call — up to 2 clears, both writes of "", never reads.
	if n := strings.Count(body, secretCleared); n == 0 || n > 2 {
		t.Errorf("secret input is cleared %d times, want 1 or 2 (after the mint call succeeds and/or fails)", n)
	}
	// No other reference to the input's value anywhere else in the
	// file beyond the read and the clear(s) already accounted for.
	total := strings.Count(body, secretRead)
	accounted := strings.Count(body, secretActuallyRead) + strings.Count(body, secretCleared)
	if total != accounted {
		t.Errorf("secret input's value is referenced %d times total but only %d are accounted for as the known read/clear sites — an unaccounted reference is exactly the kind of stray use this criterion exists to catch", total, accounted)
	}

	mintFuncSignature := `el("mint-btn").addEventListener`
	mintFuncStart := strings.Index(body, mintFuncSignature)
	if mintFuncStart < 0 {
		t.Fatalf("could not find the mint button's click handler")
	}
	// The one read of the secret must be inside the mint handler, and
	// that handler's only fetch target must be /auth/token. Delimited
	// against the next handler registration in the file, not a brace-
	// counting heuristic that would break on a reformat. Search starts
	// past the end of this handler's own signature, so it doesn't just
	// find "addEventListener" within that same signature again.
	searchFrom := mintFuncStart + len(mintFuncSignature)
	nextHandlerStart := strings.Index(body[searchFrom:], "addEventListener")
	var mintHandlerBody string
	if nextHandlerStart > 0 {
		mintHandlerBody = body[mintFuncStart : searchFrom+nextHandlerStart]
	} else {
		mintHandlerBody = body[mintFuncStart:]
	}
	if !strings.Contains(mintHandlerBody, secretRead) {
		t.Errorf("the secret input's value is not read inside the mint button's own handler — it must never be read anywhere else")
	}
	if !strings.Contains(mintHandlerBody, `"/auth/token"`) {
		t.Errorf("the mint handler doesn't call /auth/token — expected the one place the secret is used to be exactly this endpoint")
	}
	// And the reverse: /auth/token appears exactly twice in the whole
	// file — the displayed curl line and the real fetch call, both
	// inside the mint handler above — so there's no second, different
	// call site anywhere else that could also be sending the secret
	// (or anything else) to it.
	if n := strings.Count(body, `"/auth/token"`); n != 2 {
		t.Errorf(`"/auth/token" appears %d times in the embedded page, want exactly 2 (the displayed curl line and the real fetch, both in the mint handler)`, n)
	}
}

// TestDemoRoute_NotExposedOnIssuerRouter confirms /demo does not exist
// on the issuer-mode router — this is a demo of the search/retrieve
// surface, not the issuer internals (refinement/LT-53.md's own
// acceptance criterion). ModeSweep is not tested here since
// runSweepMode (cmd/api-service/main.go) never constructs an
// http.Handler at all — there is no router for /demo to be reachable
// on in that mode, by construction, not by a check that could
// regress.
func TestDemoRoute_NotExposedOnIssuerRouter(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	cfg := config.Config{}
	router := NewIssuerRouter(cfg, &api.SigningKey{Private: key, Kid: "test-kid"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("status = %d, want anything but 200 — /demo must not be reachable on the issuer router", w.Code)
	}
}

// TestDemoRoute_NotExposedOnPlainRouter covers NewRouter's own two
// modes (ModeIssuer's placeholder route, and the unrecognized-mode
// fallback) — neither of which is NewVerifierRouter, so /demo must be
// absent from both.
func TestDemoRoute_NotExposedOnPlainRouter(t *testing.T) {
	for _, mode := range []Mode{ModeIssuer, Mode("unrecognized")} {
		router := NewRouter("api-service", mode)
		req := httptest.NewRequest(http.MethodGet, "/demo", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code == http.StatusOK {
			t.Errorf("mode %q: status = %d, want anything but 200 — /demo must only exist on NewVerifierRouter", mode, w.Code)
		}
	}
}
