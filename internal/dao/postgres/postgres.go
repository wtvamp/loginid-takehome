// Package postgres implements internal/dao's Repository composite for
// both PostgreSQL and CockroachDB, per 05-data-ops/multi-db-strategy.md.
// One package serves both engines; the retry seam in retry.go is what
// makes that safe (§4, "The write-path retry seam").
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"loginid-takehome/internal/dao"
)

// engine distinguishes the two driver strings that select this package —
// "postgres" and "cockroachdb" — because withRetry needs to know which one
// it's talking to (§4: "That field is the entire reason the two driver
// strings are distinct despite selecting the same package").
type engine string

const (
	enginePostgres  engine = "postgres"
	engineCockroach engine = "cockroachdb"
)

type repository struct {
	db     *sql.DB
	engine engine
}

// New opens a connection pool against dsn for the given engine ("postgres"
// or "cockroachdb"). Registered with internal/dao's factory in register.go.
func New(dsn string, e engine) (dao.Repository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: opening connection: %w", err)
	}
	return &repository{db: db, engine: e}, nil
}

func (r *repository) Profiles() dao.ProfileRepository       { return profileRepo{r} }
func (r *repository) Credentials() dao.CredentialRepository { return credentialRepo{r} }
func (r *repository) Methods() dao.AuthMethodRepository     { return authMethodRepo{r} }

func (r *repository) Close() error { return r.db.Close() }

// withRetry runs fn inside a transaction, retrying on a genuine SQLSTATE
// 40001 (serialization failure) only when r.engine is "cockroachdb" — on
// "postgres" it runs fn exactly once. See retry.go.
func (r *repository) withRetry(ctx context.Context, fn func(*sql.Tx) error) error {
	return withRetry(ctx, r.db, r.engine, fn)
}
