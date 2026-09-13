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
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: opening connection: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("sqlite: enabling foreign_keys: %w", err)
	}
	// SQLite allows only one writer at a time; a single connection avoids
	// SQLITE_BUSY from this package's own concurrent use within a process.
	db.SetMaxOpenConns(1)
	return &repository{db: db}, nil
}

func (r *repository) Profiles() dao.ProfileRepository       { return profileRepo{r} }
func (r *repository) Credentials() dao.CredentialRepository { return credentialRepo{r} }
func (r *repository) Methods() dao.AuthMethodRepository     { return authMethodRepo{r} }

func (r *repository) Close() error { return r.db.Close() }
