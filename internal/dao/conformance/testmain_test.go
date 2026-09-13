package conformance

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"testing"
)

// testPostgresDBDSN/testCockroachDBDSN are this package's throwaway
// per-run databases (see TestMain below) — never TEST_POSTGRES_DSN/
// TEST_COCKROACHDB_DSN directly. Empty if the corresponding env var was
// never set; newPostgresFamilyFixture reads these instead of
// os.Getenv, and skips if empty, same as before this fix.
var (
	testPostgresDBDSN  string
	testCockroachDBDSN string
)

// dsnVarForDriver maps this package's own driver strings to the
// package-level throwaway-DSN variable newPostgresFamilyFixture should
// read, so it doesn't need its own if/else on driver name beyond this
// one lookup.
func dsnVarForDriver(driver string) *string {
	switch driver {
	case "postgres":
		return &testPostgresDBDSN
	case "cockroachdb":
		return &testCockroachDBDSN
	default:
		return nil
	}
}

// TestMain creates one throwaway database PER BACKEND for this whole
// package's test run (Postgres and CockroachDB independently, since a
// local run might only have one of the two DSNs configured) and drops
// each on exit.
//
// Fixes a real cross-PACKAGE race Theo found running this suite for
// real in CI (run 34761945847): this package and internal/dao/postgres
// both pointed at the SAME TEST_POSTGRES_DSN database, and `go test
// ./...` runs packages concurrently — two packages' migrations both ran
// `CREATE EXTENSION IF NOT EXISTS pg_trgm` against the same database at
// once (IF NOT EXISTS is not atomic across sessions in Postgres), so
// the loser saw "duplicate key value violates unique constraint
// pg_extension_name_index" and then "operator class gin_trgm_ops does
// not exist" (its own create lost the race too). This package's
// existing per-TEST schema isolation (newPostgresFamilyFixture's CREATE
// SCHEMA) already prevented same-package collisions; it never prevented
// cross-package ones, since pg_extension is database-wide, not
// schema-scoped.
//
// Ruled fix (PM, live-review finding): a per-package throwaway
// database, not `go test -p 1` — a flag that happens to serialize
// package execution is a hope about test-runner internals, not a
// structural guarantee, and would silently mask this exact bug for
// anyone running the suite locally with the default parallelism.
// CockroachDB supports CREATE DATABASE the same way Postgres does, so
// the same mechanism covers both backends this package tests against a
// real server for.
func TestMain(m *testing.M) {
	var cleanups []func()
	exit := func(code int) {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
		os.Exit(code)
	}

	for _, spec := range []struct {
		envVar string
		prefix string
		dsn    *string
	}{
		{"TEST_POSTGRES_DSN", "conformance_pg", &testPostgresDBDSN},
		{"TEST_COCKROACHDB_DSN", "conformance_crdb", &testCockroachDBDSN},
	} {
		adminDSN := os.Getenv(spec.envVar)
		if adminDSN == "" {
			continue // that backend's own fixture skips individually, as before
		}
		dbName, dsn, cleanup, err := createThrowawayDatabase(adminDSN, spec.prefix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "conformance package TestMain: creating throwaway database for %s: %v\n", spec.envVar, err)
			exit(1)
			return
		}
		*spec.dsn = dsn
		cleanups = append(cleanups, func() { cleanup(dbName) })
	}

	code := m.Run()
	exit(code)
}

// createThrowawayDatabase connects to whatever database adminDSN names
// (CREATE DATABASE doesn't require a specific admin database, just a
// connection with the privilege to run it, and it can't run inside a
// transaction block, which a single Exec on a fresh *sql.DB connection
// never is), creates a new, uniquely-named database, and returns a DSN
// pointing at it plus a cleanup func that drops it.
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
		// Deliberately NOT "WITH (FORCE)" (Postgres 13+ only) — this
		// function is shared by both the postgres and cockroachdb
		// fixtures, and CockroachDB doesn't recognize that option (it
		// would fail the drop with a syntax error, not silently ignore
		// it). Relies on every per-test connection to this database
		// already being closed via its own t.Cleanup by the time
		// TestMain's cleanup runs here — m.Run() only returns after
		// every test's t.Cleanup callbacks have already executed.
		if _, err := admin2.Exec("DROP DATABASE IF EXISTS " + name); err != nil {
			fmt.Fprintf(os.Stderr, "dropping throwaway database %q: %v\n", name, err)
		}
	}
	return dbName, newDSN, cleanup, nil
}

// withDatabaseName returns dsn with its database name (the path
// component of a postgres:// URL) replaced by name. Only the URL form
// is handled — a non-URL DSN is rejected rather than guessed at, since a
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
