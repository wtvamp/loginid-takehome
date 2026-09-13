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

	srv := httptest.NewServer(NewRouter("api-service"))
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
