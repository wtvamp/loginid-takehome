package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakeErr constructs a *pgconn.PgError with the given SQLSTATE, the way a
// real driver error would arrive, without needing a live DB.
func fakeErr(code string) error {
	return &pgconn.PgError{Code: code}
}

func TestRetryLoop_PostgresCallsOnce(t *testing.T) {
	calls := 0
	err := retryLoop(enginePostgres, func() error {
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
	calls := 0
	err := retryLoop(engineCockroach, func() error {
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
	calls := 0
	err := retryLoop(engineCockroach, func() error {
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
	calls := 0
	err := retryLoop(engineCockroach, func() error {
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
