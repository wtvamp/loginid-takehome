package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrationSQL reads a goose-format migration file and returns just the Up
// section — the real deliverable in migrations/sqlite/, not a duplicated
// hand-maintained fixture.
func migrationSQL(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration %s: %v", path, err)
	}
	content := string(b)
	upStart := strings.Index(content, "-- +goose Up")
	downStart := strings.Index(content, "-- +goose Down")
	if upStart == -1 || downStart == -1 {
		t.Fatalf("migration %s missing +goose Up/Down markers", path)
	}
	return content[upStart+len("-- +goose Up") : downStart]
}

// setupDB opens a fresh in-memory SQLite database per test and applies the
// real migration files — no external service needed, unlike the Postgres
// package, so every test here runs for real, always.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	// Go through the same DSN pragma New() uses, not a manual PRAGMA exec —
	// so a regression in New()'s configuration shows up here too, instead
	// of this harness silently proving a guarantee production doesn't
	// actually have (05's amendment A3).
	db, err := sql.Open("sqlite", withForeignKeysOn(":memory:"))
	if err != nil {
		t.Fatalf("opening in-memory sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	repoRoot := repoRootFromThisFile(t)
	sqliteMigration := migrationSQL(t, filepath.Join(repoRoot, "migrations", "sqlite", "00001_initial_schema.sql"))
	if _, err := db.Exec(sqliteMigration); err != nil {
		t.Fatalf("applying sqlite migration: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO auth_method (id, name, requires_secret, is_active, created_at)
		VALUES ('00000000-0000-0000-0000-000000000001', 'password', 1, 1, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seeding auth_method: %v", err)
	}

	return db
}

func repoRootFromThisFile(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..", "..")
}

func seedAuthMethodID() string { return "00000000-0000-0000-0000-000000000001" }
