// Package sqlite implements internal/dao's Repository composite for
// SQLite, per 05-data-ops/multi-db-strategy.md. A separate package from
// postgres — SQLite diverges enough (no native UUID/BOOLEAN, no trigram
// index, app-side ID generation) that forcing it through the Postgres
// implementation would mean littering that code with backend
// conditionals, which is the exact failure mode a repository-per-backend
// split is meant to avoid.
package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"loginid-takehome/internal/dao"
)

type repository struct {
	db *sql.DB
}

// New opens a connection against dsn (a file path, or ":memory:").
// Registered with internal/dao's factory in register.go. There is no
// retry wrapper here at all — not a no-op shim. SQLite has no
// retryable-serialization error class, so a no-op would be ceremony
// implying a shared abstraction that does not exist (contract §4).
func New(dsn string) (dao.Repository, error) {
	// foreign_keys is per-CONNECTION, not a property of the database file
	// — a bare `PRAGMA foreign_keys = ON` exec after opening only covers
	// the one connection that happened to run it, and silently stops
	// covering anything the moment the pool acquires a second connection
	// (a raised MaxOpenConns, a reconnect after an error, ...). Requesting
	// it via the DSN's `_foreign_keys` query param makes modernc.org/sqlite
	// apply it to every connection it opens, by construction (05's
	// amendment A3, surfaced during PR #8 review). SetMaxOpenConns(1)
	// below is kept for its own, separate reason (SQLite's single-writer
	// model) — it must not be the only thing keeping this guarantee true.
	dsn = withForeignKeysOn(dsn)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening connection: %w", err)
	}
	// SetMaxOpenConns(1) correctly stops this package's own goroutines
	// from hitting SQLITE_BUSY against each other — but it does not
	// remove contention, it moves it into database/sql's pool queue.
	// Under concurrent callers (e.g. api-service handling concurrent
	// requests), anything holding this one connection (a large
	// DeleteExpired sweep, a multi-statement CreateProfileWithCredential)
	// blocks every other concurrent caller until it releases or the
	// caller's own context deadline fires — surfacing as a silent
	// context.DeadlineExceeded, indistinguishable from a slow query
	// (Nolan Reyes, PR #8 review). This is a correct, cheap
	// simplification for the tier SQLite is scoped to here: local/dev/
	// demo, never a concurrent-production backend
	// (multi-db-strategy.md, "What Search does on SQLite" — "SQLite is
	// the local/dev/demo backend, not a production peer"). If that scope
	// ever changes, this connection-pool ceiling needs revisiting first.
	db.SetMaxOpenConns(1)
	return &repository{db: db}, nil
}

func withForeignKeysOn(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_foreign_keys=on"
}

func (r *repository) Profiles() dao.ProfileRepository       { return profileRepo{r} }
func (r *repository) Credentials() dao.CredentialRepository { return credentialRepo{r} }
func (r *repository) Methods() dao.AuthMethodRepository     { return authMethodRepo{r} }

func (r *repository) Close() error { return r.db.Close() }
