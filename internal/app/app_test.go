package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthz is the integration test LT-34's acceptance criteria names as
// the enforcement site for the health endpoint's response shape.
func TestHealthz(t *testing.T) {
	Commit = "abc123"

	srv := httptest.NewServer(NewRouter("api-service", ModeVerifier))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status field = %v, want %q", body["status"], "ok")
	}
	if body["service"] != "api-service" {
		t.Errorf("service field = %v, want %q", body["service"], "api-service")
	}
	if build, _ := body["build"].(string); build == "" {
		t.Errorf("build field must be non-empty, got %v", body["build"])
	}

	// Per 02's ruling (api-auth-design.md): no version string, timestamp,
	// or any other field belongs on this unauthenticated endpoint.
	allowed := map[string]bool{"status": true, "service": true, "build": true}
	for k := range body {
		if !allowed[k] {
			t.Errorf("unexpected field %q in health response — only status/service/build are permitted on this unauthenticated endpoint", k)
		}
	}
}

// TestNewRouter_IssuerModeAddsTokenRoute is LT-32's acceptance-criteria
// enforcement site for "internal/app branches on it at startup": the
// issuer and verifier routers must be demonstrably different handlers, not
// just two identical routers reading a value neither acts on.
func TestNewRouter_IssuerModeAddsTokenRoute(t *testing.T) {
	srv := httptest.NewServer(NewRouter("api-service", ModeIssuer))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/token", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /auth/token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 501, not 404: the route must exist and be reachable on the issuer
	// router even though S7/LT-40 hasn't implemented real issuance yet —
	// a 404 here would mean the route isn't actually wired.
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d (route must exist on the issuer router)", resp.StatusCode, http.StatusNotImplemented)
	}
}

// TestNewRouter_VerifierModeHasNoTokenRoute is the other half of the same
// criterion: the verifying router — the one actually deployed as
// api-service, per LT-32's physical-absence requirement — must not expose
// the issuance route at all, not even as a 501. A 404 here is the correct
// outcome, mirroring "physically absent, not just RBAC-denied" one layer
// up, at the HTTP surface rather than the Kubernetes manifest.
func TestNewRouter_VerifierModeHasNoTokenRoute(t *testing.T) {
	srv := httptest.NewServer(NewRouter("api-service", ModeVerifier))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/token", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /auth/token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (verifier router must not expose the issuance route)", resp.StatusCode, http.StatusNotFound)
	}
}

// TestNewRouter_UnrecognizedModeFallsBackToVerifier asserts NewRouter
// doesn't rely solely on config.Load's validation never being bypassed
// (Oren Castellan, PR #18 review): an unrecognized Mode value must still
// produce the safer of the two route sets (no issuance route) rather than
// silently matching ModeIssuer's behavior by accident of comparison.
func TestNewRouter_UnrecognizedModeFallsBackToVerifier(t *testing.T) {
	srv := httptest.NewServer(NewRouter("api-service", Mode("bogus")))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/auth/token", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /auth/token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d (unrecognized mode must not expose the issuance route)", resp.StatusCode, http.StatusNotFound)
	}
}
