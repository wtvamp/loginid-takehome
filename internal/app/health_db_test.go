package app

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // registers "sqlite" for these tests only
)

// TestHealthzWithDB_Reachable_ReportsOK exercises the real path: a live
// *sql.DB that actually answers PingContext.
func TestHealthzWithDB_Reachable_ReportsOK(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	srv := httptest.NewServer(healthHandlerWithDB("api-service", db))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (must stay green regardless of db)", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["db"] != "ok" {
		t.Errorf("db = %v, want ok", body["db"])
	}
}

// TestHealthzWithDB_Unreachable_ReportsDownNotError proves the failure
// path — a *sql.DB pointed at a DSN that can never connect — reports
// "down" (not an error status), and that the response body never leaks
// the DSN or the underlying driver error text. This is LT-52 criterion
// 5's own named enforcement site.
func TestHealthzWithDB_Unreachable_ReportsDownNotError(t *testing.T) {
	const secretDSN = "file:/definitely/does/not/exist/nowhere.db?_pragma=busy_timeout(1)&mode=ro"
	db, err := sql.Open("sqlite", secretDSN)
	if err != nil {
		t.Fatalf("opening sqlite (should succeed — sql.Open never dials): %v", err)
	}
	defer func() { _ = db.Close() }()

	srv := httptest.NewServer(healthHandlerWithDB("api-service", db))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when db is unreachable — /healthz must stay green", resp.StatusCode)
	}

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	raw := string(rawBytes)

	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok (db down must not fail the process's own liveness)", body["status"])
	}
	if body["db"] != "down" {
		t.Errorf("db = %v, want down", body["db"])
	}

	if strings.Contains(raw, "nowhere.db") || strings.Contains(raw, secretDSN) {
		t.Errorf("response body leaks the DSN: %s", raw)
	}
	for _, leak := range []string{"unable to open database file", "no such file", "sqlite:"} {
		if strings.Contains(strings.ToLower(raw), strings.ToLower(leak)) {
			t.Errorf("response body leaks driver error detail (%q): %s", leak, raw)
		}
	}
}

// TestHealthzWithDB_NilDB_ReportsDown covers the startup-time case:
// cmd/api-service/main.go never opened a ping handle at all (no
// DB_DRIVER/DB_DSN configured, or DB_DRIVER unrecognized).
func TestHealthzWithDB_NilDB_ReportsDown(t *testing.T) {
	srv := httptest.NewServer(healthHandlerWithDB("api-service", nil))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["db"] != "down" {
		t.Errorf("db = %v, want down (nil ping handle)", body["db"])
	}
}
