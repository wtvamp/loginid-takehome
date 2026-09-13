package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// sqlstateSerializationFailure is the SQLSTATE CockroachDB (and Postgres,
// though Postgres at READ COMMITTED never produces it) returns when a
// transaction must be retried, per multi-db-strategy.md §4.
const sqlstateSerializationFailure = "40001"

const maxRetries = 5

// withRetry is the unexported helper inside this package that every write
// method runs its statement inside (§4, "Where the wrapper sits"). It is
// NOT in the composite and NOT above the interface boundary — it wraps a
// single transaction attempt and retries only when engine is
// "cockroachdb" and the failure is a genuine SQLSTATE 40001. On
// "postgres" it calls fn exactly once. Search never calls this — it's
// read-only and never encounters 40001 (§4).
func withRetry(ctx context.Context, db *sql.DB, e engine, fn func(*sql.Tx) error) error {
	return retryLoop(e, func() error {
		return runOnce(ctx, db, fn)
	})
}

// retryLoop is the pure engine-conditional retry policy, separated from
// transaction management so it's unit-testable without a live *sql.DB.
func retryLoop(e engine, attempt func() error) error {
	attempts := 1
	if e == engineCockroach {
		attempts = maxRetries
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		lastErr = attempt()
		if lastErr == nil {
			return nil
		}
		if e != engineCockroach || !isRetryable(lastErr) {
			return lastErr
		}
		// Retryable on CockroachDB: loop and try again. No backoff sleep
		// here — a serialization failure is not a load signal the way a
		// connection error is, and 05's contract does not ask for one;
		// keeping the retry bounded (maxRetries) is what prevents an
		// unbounded loop.
	}
	return lastErr
}

func runOnce(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// isRetryable reports whether err is a genuine SQLSTATE 40001, not a
// string match on any error message.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == sqlstateSerializationFailure
	}
	return false
}
