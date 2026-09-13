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
