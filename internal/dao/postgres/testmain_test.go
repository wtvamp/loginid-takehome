package postgres

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"testing"
)

// testDBDSN is the DSN this package's tests connect to — a throwaway
// database TestMain creates for THIS test binary's run only, never the
// database TEST_POSTGRES_DSN itself names. Empty if TEST_POSTGRES_DSN
// was never set (local run, no database configured); tests read this
// var (via requireTestDB) instead of os.Getenv directly.
var testDBDSN string

// TestMain creates one throwaway database for this whole package's test
// run and drops it on exit — fixing a real cross-PACKAGE race Theo found
// running this suite for real in CI (run 34761945847): internal/dao/postgres
// and internal/dao/conformance both pointed at the SAME TEST_POSTGRES_DSN
// database, and `go test ./...` runs packages concurrently, so two
// packages' migrations both ran `CREATE EXTENSION IF NOT EXISTS pg_trgm`
// against the same database at once — IF NOT EXISTS is not atomic across
// sessions in Postgres, so the loser saw "duplicate key value violates
// unique constraint pg_extension_name_index" and then "operator class
// gin_trgm_ops does not exist" (the extension's objects were never
// installed because ITS OWN create lost the race too). This package's
// existing per-TEST schema isolation (setupDB's CREATE SCHEMA) already
// prevented same-package collisions; it never prevented cross-package
// ones, because both packages' migrations landed in the same physical
// database's shared catalogs (pg_extension is database-wide, not
// schema-scoped) regardless of which schema each test used.
//
// Ruled fix (PM, live-review finding): a per-package throwaway database,
// not `go test -p 1` — a flag that happens to serialize package
// execution is a hope about test-runner internals, not a structural
// guarantee, and it would silently mask this exact bug for anyone
// running the suite locally with the default parallelism.
func TestMain(m *testing.M) {
	adminDSN := os.Getenv("TEST_POSTGRES_DSN")
	if adminDSN == "" {
		// Unset: every test's own requireTestDB call skips (or, once
		// CI-aware, fails) individually — unchanged from before this
		// fix. No throwaway database to create or tear down.
		os.Exit(m.Run())
	}

	dbName, dsn, cleanup, err := createThrowawayDatabase(adminDSN, "postgres_pkg")
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres package TestMain: creating throwaway database: %v\n", err)
		os.Exit(1)
	}
	testDBDSN = dsn

	code := m.Run()
	cleanup(dbName)
	os.Exit(code)
}

// createThrowawayDatabase connects to whatever database adminDSN names
// (any reachable database on the target server works — CREATE DATABASE
// doesn't require a specific admin database, just a connection with the
// privilege to run it, and it can't run inside a transaction block,
// which a single Exec on a fresh *sql.DB connection never is), creates a
// new, uniquely-named database, and returns a DSN pointing at it plus a
// cleanup func that drops it. namePrefix distinguishes which package's
// throwaway database this is, for anyone inspecting the server's
// database list mid-run.
func createThrowawayDatabase(adminDSN, namePrefix string) (dbName, newDSN string, cleanup func(dbName string), err error) {
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return "", "", nil, fmt.Errorf("opening admin connection: %w", err)
	}
	defer func() { _ = admin.Close() }()

	dbName = fmt.Sprintf("test_%s_%d", namePrefix, rand.Int63())
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		return "", "", nil, fmt.Errorf("creating database %q: %w", dbName, err)
	}

	newDSN, err = withDatabaseName(adminDSN, dbName)
	if err != nil {
		return "", "", nil, fmt.Errorf("building DSN for %q: %w", dbName, err)
	}

	cleanup = func(name string) {
		admin2, err := sql.Open("pgx", adminDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dropping throwaway database %q: reopening admin connection: %v\n", name, err)
			return
		}
		defer func() { _ = admin2.Close() }()
		// Postgres refuses DROP DATABASE while any connection to it is
		// still open — WITH (FORCE) (Postgres 13+) terminates them
		// first rather than requiring this harness to track and close
		// every *sql.DB it ever handed out.
		if _, err := admin2.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); err != nil {
			fmt.Fprintf(os.Stderr, "dropping throwaway database %q: %v\n", name, err)
		}
	}
	return dbName, newDSN, cleanup, nil
}

// withDatabaseName returns dsn with its database name (the path
// component of a postgres:// URL, or the dbname key of a libpq
// keyword/value DSN) replaced by name. Only the URL form is handled
// directly; a non-URL DSN is rejected rather than guessed at, since a
// wrong guess here would silently point tests at the WRONG database
// rather than failing loudly.
func withDatabaseName(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing DSN as a URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("DSN scheme %q is not postgres:// or postgresql:// — libpq keyword/value DSNs aren't supported by this test harness", u.Scheme)
	}
	u.Path = "/" + name
	return u.String(), nil
}
