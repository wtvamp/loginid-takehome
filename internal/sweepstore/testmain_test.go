package sweepstore

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	_ "modernc.org/sqlite"             // registers the "sqlite" database/sql driver
)

// testDBDSN is this package's own throwaway Postgres database for its
// test run — the same pattern internal/dao/postgres/testmain_test.go
// established (PR #49), for the same reason: two test packages sharing
// one physical database can race on database-wide catalog operations.
// This package's own migration doesn't touch pg_extension, but the
// isolation itself (never sharing a physical database with another
// test package) is the right default regardless of which specific race
// prompted it originally. Empty when TEST_POSTGRES_DSN is unset.
var testDBDSN string

func TestMain(m *testing.M) {
	adminDSN := os.Getenv("TEST_POSTGRES_DSN")
	if adminDSN == "" {
		os.Exit(m.Run())
	}

	dbName, dsn, cleanup, err := createThrowawayDatabase(adminDSN, "sweepstore_pkg")
	if err != nil {
		fmt.Fprintf(os.Stderr, "sweepstore package TestMain: creating throwaway database: %v\n", err)
		os.Exit(1)
	}
	testDBDSN = dsn

	code := m.Run()
	cleanup(dbName)
	os.Exit(code)
}

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
		if _, err := admin2.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)"); err != nil {
			fmt.Fprintf(os.Stderr, "dropping throwaway database %q: %v\n", name, err)
		}
	}
	return dbName, newDSN, cleanup, nil
}

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
