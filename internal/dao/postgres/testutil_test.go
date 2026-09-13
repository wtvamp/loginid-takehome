package postgres

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// migrationSQL reads a goose-format migration file and returns just the Up
// section's SQL — the real deliverable in migrations/{shared,postgres}/,
// not a duplicated copy of the schema. Applying the actual migration files
// in tests is what keeps this test harness honest: if the migrations
// drift from what the Go code expects, the integration tests fail, not
// just a hand-maintained fixture.
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

// splitSQLStatements splits a migration's Up section into individual
// statements on ";" — used only for migrations that must NOT run as one
// multi-statement transaction (see the 00002 phone-CHECK migration's own
// comment). "--" comment lines are stripped first: this file's own
// prose comments use ordinary English semicolons ("...split; see the
// cockroachdb migration's own comment."), which a naive split-on-";"
// would otherwise cut mid-sentence and misidentify as a statement
// boundary. None of the actual SQL statements' string literals contain a
// ";", so splitting the comment-free text is safe — this is not a
// general-purpose SQL parser.
func splitSQLStatements(sqlText string) []string {
	var codeOnly strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		if idx := strings.Index(line, "--"); idx != -1 {
			line = line[:idx]
		}
		codeOnly.WriteString(line)
		codeOnly.WriteString("\n")
	}

	var out []string
	for _, stmt := range strings.Split(codeOnly.String(), ";") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}

// setupDB connects to this package's throwaway per-run database (see
// TestMain in testmain_test.go — testDBDSN, not TEST_POSTGRES_DSN
// directly), skipping the test if no database is configured at all
// LOCALLY (CI unset), but FAILING it (not skipping) if CI is set and
// testDBDSN is still empty anyway — GitHub Actions sets CI=true
// unconditionally, so this is a reliable signal this test is running in
// the gate, not on a developer's machine. Naomi Voss's joint-review
// finding: this fixture's plain t.Skip made every Postgres/CockroachDB
// conformance and integration test look green in CI while never
// actually running against a real database at all — pr-check never set
// these DSNs, so "passing" meant "skipped," indistinguishable from a
// real pass in the CI summary. A skipped assertion that reads as a pass
// is worse than a red build (05's own §7 principle). Applies the real
// migration files into a fresh, uniquely-named schema, and tears the
// schema down on test cleanup so tests never collide with each other
// WITHIN this package (cross-package collisions are what TestMain's
// throwaway database prevents).
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := testDBDSN
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("TEST_POSTGRES_DSN not set in CI — the conformance/integration gate is not running")
		}
		t.Skip("TEST_POSTGRES_DSN not set — skipping Postgres integration test (see refinement/LT-36-LT-37.md: real-backend verification is this package's responsibility, run manually or in CI once wired)")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening TEST_POSTGRES_DSN: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Cap the pool at one connection BEFORE anything session-scoped
	// (SET search_path) runs — otherwise the migration or a later test
	// query can land on a different pooled connection that never saw the
	// SET, and silently falls back to the default search_path.
	db.SetMaxOpenConns(1)

	schemaName := fmt.Sprintf("test_%d", rand.Int63())
	if _, err := db.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("creating test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec("DROP SCHEMA " + schemaName + " CASCADE")
	})
	// Include "public" on the path: pg_trgm's CREATE EXTENSION IF NOT
	// EXISTS in the migration is a no-op after the first test schema
	// creates it, and an extension's objects (gin_trgm_ops) live in
	// whichever schema was current at creation time — later test schemas
	// need "public" on their path to see it, not just their own name.
	if _, err := db.Exec("SET search_path TO " + schemaName + ", public"); err != nil {
		t.Fatalf("setting search_path: %v", err)
	}

	repoRoot := repoRootFromThisFile(t)
	postgresMigration := migrationSQL(t, filepath.Join(repoRoot, "migrations", "postgres", "00001_initial_schema.sql"))
	if _, err := db.Exec(postgresMigration); err != nil {
		t.Fatalf("applying postgres migration: %v", err)
	}
	// 00002: contract amendment A6's phone CHECK narrowing — a real goose
	// migration, applied here the same way the real pipeline would apply
	// it in sequence, not folded back into 00001's text (that would be a
	// no-op against any database that already applied 00001). Applied
	// statement-by-statement, not as one multi-statement Exec: CockroachDB
	// rejects an ADD CONSTRAINT reusing a name a DROP CONSTRAINT removed
	// earlier in the same implicit transaction ("duplicate constraint
	// name"), so each ALTER needs to commit before the next one runs —
	// the migration file itself carries a matching "-- +goose NO
	// TRANSACTION" directive for when goose (not this harness) applies it
	// for real.
	phoneNarrowing := migrationSQL(t, filepath.Join(repoRoot, "migrations", "postgres", "00002_phone_e164_narrow_range.sql"))
	for _, stmt := range splitSQLStatements(phoneNarrowing) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("applying postgres migration 00002 statement %q: %v", stmt, err)
		}
	}
	sharedSeed := migrationSQL(t, filepath.Join(repoRoot, "migrations", "shared", "00001_seed_auth_method.sql"))
	if _, err := db.Exec(sharedSeed); err != nil {
		t.Fatalf("applying shared seed migration: %v", err)
	}

	return db
}

func repoRootFromThisFile(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// this package: <repoRoot>/internal/dao/postgres
	return filepath.Join(wd, "..", "..", "..")
}

// seedAuthMethodID returns the id of the 'password' method seeded by the
// shared migration, for tests that need a valid method_id FK target.
func seedAuthMethodID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow("SELECT id FROM auth_method WHERE name = 'password'").Scan(&id); err != nil {
		t.Fatalf("looking up seeded auth_method: %v", err)
	}
	return id
}
