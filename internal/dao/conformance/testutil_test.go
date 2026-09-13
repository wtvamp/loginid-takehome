// Package conformance is LT-38: 05's §7 requirement that behavioral
// conformance (do the backends BEHAVE alike) is proven separately from
// signature conformance (LT-35: do the Go shapes match). Two
// implementations can match every declaration and still return different
// rows for the same call — every guarantee below is asserted as a named
// test case against all three backends from one table-driven suite.
package conformance

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"loginid-takehome/internal/dao"
	_ "loginid-takehome/internal/dao/postgres" // registers "postgres" and "cockroachdb"
	_ "loginid-takehome/internal/dao/sqlite"   // registers "sqlite"
)

// fixture is everything one backend's test run needs: the Repository
// under test, plus an admin *sql.DB for the handful of operations the
// Repository interface deliberately doesn't expose (creating/deactivating
// auth_method rows — that table is read-only through the interface per
// the contract; writes to it are "an operational action", not part of the
// normal request path).
type fixture struct {
	driver string
	repo   dao.Repository
	admin  *sql.DB
}

// backend is one target this suite runs every guarantee against.
type backend struct {
	driver string // dao.New's driver argument
	newFix func(t *testing.T) fixture
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// this file: <repoRoot>/internal/dao/conformance/testutil_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

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
// statements on ";" — used for migrations that must not run as one
// multi-statement transaction (see the 00002 phone-CHECK migration's own
// comment: CockroachDB rejects an ADD CONSTRAINT reusing a name a DROP
// CONSTRAINT removed earlier in the same implicit transaction). "--"
// comment lines are stripped first: this file's own prose comments use
// ordinary English semicolons, which a naive split-on-";" would otherwise
// cut mid-sentence and misidentify as a statement boundary. None of the
// actual SQL statements' string literals contain a ";", so splitting the
// comment-free text is safe — this is not a general-purpose SQL parser.
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

// newPostgresFamilyFixture is shared by the "postgres" and "cockroachdb"
// backends — same migration, same setup, different env var and driver
// string. Reads this package's own throwaway per-run database (see
// TestMain in testmain_test.go — dsnVarForDriver, not the env var
// directly), skipping the test if that backend's DSN was never set at
// all LOCALLY (CI unset), but FAILING it (not skipping) if CI is set and
// the DSN is still absent — GitHub Actions sets CI=true unconditionally,
// so this is a reliable signal this test is running in the gate, not on
// a developer's machine. Naomi Voss's joint-review finding: this
// fixture's plain t.Skipf made every Postgres/CockroachDB conformance
// test (including TestConformance_CockroachDB_RealSerializationRetry)
// look green in CI while never actually running against a real
// database — pr-check never set these DSNs, so "passing" meant
// "skipped," indistinguishable from a real pass in the CI summary. A
// skipped assertion that reads as a pass is worse than a red build
// (05's own §7 principle).
func newPostgresFamilyFixture(t *testing.T, driver, dsnEnvVar string) fixture {
	t.Helper()
	dsnVar := dsnVarForDriver(driver)
	if dsnVar == nil || *dsnVar == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s not set in CI — the %s conformance gate is not running", dsnEnvVar, driver)
		}
		t.Skipf("%s not set — skipping %s conformance run", dsnEnvVar, driver)
	}
	dsn := *dsnVar

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening %s: %v", dsnEnvVar, err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	admin.SetMaxOpenConns(1)

	schemaName := fmt.Sprintf("conf_%s_%d", driver, rand.Int63())
	if _, err := admin.Exec("CREATE SCHEMA " + schemaName); err != nil {
		t.Fatalf("creating schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schemaName + " CASCADE") })
	if _, err := admin.Exec("SET search_path TO " + schemaName + ", public"); err != nil {
		t.Fatalf("setting search_path: %v", err)
	}

	root := repoRoot(t)
	// driver selects the migration directory (postgres/ or
	// cockroachdb/) — split under amendment A5 after COLLATE "C" proved
	// invalid syntax on CockroachDB; everything else about this fixture
	// is identical between the two engines.
	if _, err := admin.Exec(migrationSQL(t, filepath.Join(root, "migrations", driver, "00001_initial_schema.sql"))); err != nil {
		t.Fatalf("applying %s migration: %v", driver, err)
	}
	// 00002: contract amendment A6's phone CHECK narrowing — applied in
	// sequence like the real pipeline would, not folded back into 00001
	// (which would be a no-op against an already-migrated database).
	// Statement-by-statement, not one multi-statement Exec: CockroachDB
	// rejects an ADD CONSTRAINT reusing a name a DROP CONSTRAINT removed
	// earlier in the same implicit transaction.
	phoneNarrowing := migrationSQL(t, filepath.Join(root, "migrations", driver, "00002_phone_e164_narrow_range.sql"))
	for _, stmt := range splitSQLStatements(phoneNarrowing) {
		if _, err := admin.Exec(stmt); err != nil {
			t.Fatalf("applying %s migration 00002 statement %q: %v", driver, stmt, err)
		}
	}
	if _, err := admin.Exec(migrationSQL(t, filepath.Join(root, "migrations", "shared", "00001_seed_auth_method.sql"))); err != nil {
		t.Fatalf("applying shared seed migration: %v", err)
	}

	// dao.New opens its OWN connection via the driver string, which won't
	// have this schema's search_path set. Give it a DSN carrying the
	// search_path via connection options so every query the repository
	// issues lands in the isolated test schema too.
	scopedDSN := dsn
	if strings.Contains(scopedDSN, "?") {
		scopedDSN += "&"
	} else {
		scopedDSN += "?"
	}
	scopedDSN += "search_path=" + schemaName + ",public"

	repo, err := dao.New(driver, scopedDSN)
	if err != nil {
		t.Fatalf("dao.New(%q): %v", driver, err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	return fixture{driver: driver, repo: repo, admin: admin}
}

func newSQLiteFixture(t *testing.T) fixture {
	t.Helper()
	// dao.New("sqlite", dsn) opens its own connection; apply the migration
	// through a second connection to the SAME in-memory database via a
	// shared cache DSN, since ":memory:" alone gives each connection its
	// own private database.
	dsn := fmt.Sprintf("file:conf_%d?mode=memory&cache=shared", rand.Int63())

	admin, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("opening sqlite admin connection: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	admin.SetMaxOpenConns(1)

	root := repoRoot(t)
	if _, err := admin.Exec(migrationSQL(t, filepath.Join(root, "migrations", "sqlite", "00001_initial_schema.sql"))); err != nil {
		t.Fatalf("applying sqlite migration: %v", err)
	}
	if _, err := admin.Exec(`INSERT INTO auth_method (id, name, requires_secret, is_active, created_at)
		VALUES (?, 'password', 1, 1, '2026-01-01T00:00:00Z')`, seededMethodID); err != nil {
		t.Fatalf("seeding auth_method: %v", err)
	}

	repo, err := dao.New("sqlite", dsn)
	if err != nil {
		t.Fatalf("dao.New(sqlite): %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	// Keep the admin connection alive for the test's duration — an
	// in-memory shared-cache database disappears when its last connection
	// closes, and t.Cleanup runs in LIFO order so this runs after
	// repo.Close().

	return fixture{driver: "sqlite", repo: repo, admin: admin}
}

const seededMethodID = "00000000-0000-0000-0000-000000000001"

// backends is the list every named guarantee test iterates over.
// "cockroachdb" and "postgres" use the same setup helper with different
// env vars/driver strings, per the contract's own framing (§1: distinct
// strings selecting the same package).
var backends = []backend{
	{driver: "postgres", newFix: func(t *testing.T) fixture {
		return newPostgresFamilyFixture(t, "postgres", "TEST_POSTGRES_DSN")
	}},
	{driver: "cockroachdb", newFix: func(t *testing.T) fixture {
		return newPostgresFamilyFixture(t, "cockroachdb", "TEST_COCKROACHDB_DSN")
	}},
	{driver: "sqlite", newFix: newSQLiteFixture},
}
