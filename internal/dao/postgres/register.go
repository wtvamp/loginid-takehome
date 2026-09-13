package postgres

import "loginid-takehome/internal/dao"

// init registers this package under both driver strings, per
// multi-db-strategy.md §1: "postgres" and "cockroachdb" both select this
// package, distinct strings only so withRetry knows which engine it's
// talking to.
func init() {
	dao.Register("postgres", func(dsn string) (dao.Repository, error) {
		return New(dsn, enginePostgres)
	})
	dao.Register("cockroachdb", func(dsn string) (dao.Repository, error) {
		return New(dsn, engineCockroach)
	})
}
