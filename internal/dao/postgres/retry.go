package postgres

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// sqlstateSerializationFailure is the SQLSTATE CockroachDB (and Postgres,
// though Postgres at READ COMMITTED never produces it) returns when a
// transaction must be retried, per multi-db-strategy.md §4.
const sqlstateSerializationFailure = "40001"

const maxRetries = 5

// backoffBase/backoffJitter: a small jittered backoff between CockroachDB
// retries. Nolan Reyes's review (PR #6/#8) flagged zero backoff as a real
// production risk — colliding transactions retrying in lockstep on the
// same tick collide again, a pattern he'd seen firsthand (advisory-lock
// retries synchronized to the millisecond, fixed only once jitter was
// added). The jitter, not the base delay, is what matters: it's what
// keeps two colliding retriers from staying in lockstep.
const (
	backoffBase   = 5 * time.Millisecond
	backoffJitter = 15 * time.Millisecond
)

// backoffSleep is a package var so unit tests can stub it to a no-op —
// retryLoop's engine-conditional logic is what those tests exercise, not
// wall-clock timing. Respects ctx: a cancelled context during backoff
// returns immediately rather than sleeping out the full jittered delay.
var backoffSleep = func(ctx context.Context, attempt int) error {
	d := backoffBase + time.Duration(rand.Int63n(int64(backoffJitter)))
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// withRetry is the unexported helper inside this package that every write
// method runs its statement inside (§4, "Where the wrapper sits"). It is
// NOT in the composite and NOT above the interface boundary — it wraps a
// single transaction attempt and retries only when engine is
// "cockroachdb" and the failure is a genuine SQLSTATE 40001. On
// "postgres" it calls fn exactly once. Search never calls this — it's
// read-only and never encounters 40001 (§4).
func withRetry(ctx context.Context, db *sql.DB, e engine, fn func(*sql.Tx) error) error {
	return retryLoop(ctx, e, func() error {
		return runOnce(ctx, db, fn)
	})
}

// retryLoop is the engine-conditional retry policy, separated from
// transaction management so it's unit-testable without a live *sql.DB.
func retryLoop(ctx context.Context, e engine, attempt func() error) error {
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
		if i < attempts-1 {
			if err := backoffSleep(ctx, i); err != nil {
				// Context cancelled during backoff — return the
				// retryable error we already had, not the cancellation,
				// so the caller sees why the write actually failed.
				return lastErr
			}
		}
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
