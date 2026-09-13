package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakeErr constructs a *pgconn.PgError with the given SQLSTATE, the way a
// real driver error would arrive, without needing a live DB.
func fakeErr(code string) error {
	return &pgconn.PgError{Code: code}
}

// stubNoBackoff replaces backoffSleep with a no-op for the duration of a
// test — these tests exercise retryLoop's engine-conditional logic, not
// wall-clock timing, and 5 real jittered sleeps per test would make the
// suite slow and flaky under load for no benefit.
func stubNoBackoff(t *testing.T) {
	t.Helper()
	orig := backoffSleep
	backoffSleep = func(ctx context.Context, attempt int) error { return nil }
	t.Cleanup(func() { backoffSleep = orig })
}

func TestRetryLoop_PostgresCallsOnce(t *testing.T) {
	stubNoBackoff(t)
	calls := 0
	err := retryLoop(context.Background(), enginePostgres, func() error {
		calls++
		return fakeErr(sqlstateSerializationFailure)
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1 — postgres engine must not retry", calls)
	}
	if !errors.As(err, new(*pgconn.PgError)) {
		t.Errorf("expected the underlying error returned, got %v", err)
	}
}

func TestRetryLoop_CockroachRetriesOnSerializationFailure(t *testing.T) {
	stubNoBackoff(t)
	calls := 0
	err := retryLoop(context.Background(), engineCockroach, func() error {
		calls++
		return fakeErr(sqlstateSerializationFailure)
	})
	if calls != maxRetries {
		t.Errorf("calls = %d, want %d — cockroachdb engine must retry a genuine 40001 up to the bound", calls, maxRetries)
	}
	if err == nil {
		t.Error("expected an error after exhausting retries, got nil")
	}
}

func TestRetryLoop_CockroachDoesNotRetryNonRetryableError(t *testing.T) {
	stubNoBackoff(t)
	calls := 0
	err := retryLoop(context.Background(), engineCockroach, func() error {
		calls++
		return fakeErr(sqlstateUniqueViolation)
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1 — a non-40001 error must not trigger a retry even on cockroachdb", calls)
	}
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestRetryLoop_SucceedsWithoutRetry(t *testing.T) {
	stubNoBackoff(t)
	calls := 0
	err := retryLoop(context.Background(), engineCockroach, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetryLoop_BackoffRespectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := retryLoop(ctx, engineCockroach, func() error {
		calls++
		return fakeErr(sqlstateSerializationFailure)
	})
	// backoffSleep sees ctx already cancelled and returns immediately
	// after the first attempt, without retrying maxRetries times.
	if calls != 1 {
		t.Errorf("calls = %d, want 1 — a cancelled context should stop the retry loop after the first attempt's backoff", calls)
	}
	if err == nil {
		t.Error("expected the retryable error to be returned, not masked by the cancellation")
	}
}

func TestIsRetryable(t *testing.T) {
	if !isRetryable(fakeErr(sqlstateSerializationFailure)) {
		t.Error("40001 should be retryable")
	}
	if isRetryable(fakeErr(sqlstateUniqueViolation)) {
		t.Error("23505 should not be retryable")
	}
	if isRetryable(errors.New("some other error")) {
		t.Error("a non-pgconn error should not be retryable")
	}
}
