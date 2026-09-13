package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"loginid-takehome/internal/dao"
)

func TestTranslateError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"duplicate username", &pgconn.PgError{Code: sqlstateUniqueViolation, ConstraintName: constraintUsernameUnique}, dao.ErrDuplicateUsername},
		{"duplicate id (PK collision)", &pgconn.PgError{Code: sqlstateUniqueViolation, ConstraintName: "user_profile_pkey"}, dao.ErrAlreadyExists},
		{"unknown method_id", &pgconn.PgError{Code: sqlstateForeignKeyViolation, ConstraintName: constraintMethodFK}, dao.ErrInvalidMethod},
		{"other FK violation", &pgconn.PgError{Code: sqlstateForeignKeyViolation, ConstraintName: "some_other_fk"}, dao.ErrInvalidArgument},
		{"secret-state CHECK", &pgconn.PgError{Code: sqlstateCheckViolation, ConstraintName: constraintSecretState}, dao.ErrInvalidCredential},
		{"other CHECK violation", &pgconn.PgError{Code: sqlstateCheckViolation, ConstraintName: "ck_user_profile_phone_e164"}, dao.ErrInvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := translateError(c.err)
			if !errors.Is(got, c.want) {
				t.Errorf("translateError(%+v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestTranslateError_NeverEmbedsRawDriverText(t *testing.T) {
	// A CHECK violation's DETAIL line can echo the failing row's actual
	// column values (PII) — the translated error must never carry it.
	pgErr := &pgconn.PgError{
		Code:           sqlstateCheckViolation,
		ConstraintName: "some_unrecognized_check",
		Message:        "new row for relation \"user_profile\" violates check constraint",
		Detail:         "Failing row contains (abc, Jane Doe, +15551234567, ...).",
	}
	got := translateError(pgErr)
	if got.Error() != dao.ErrInvalidArgument.Error() {
		t.Fatalf("expected ErrInvalidArgument for an unrecognized CHECK violation, got %v", got)
	}

	// Also check the unclassified-error wrapping path never leaks Detail.
	unrecognized := &pgconn.PgError{
		Code:    "99999",
		Message: "some message",
		Detail:  "Key (username)=(secret-value) already exists.",
	}
	wrapped := translateError(unrecognized)
	if wrapped == nil {
		t.Fatal("expected a wrapped error")
	}
	if got := wrapped.Error(); containsAny(got, []string{"secret-value", unrecognized.Detail, unrecognized.Message}) {
		t.Errorf("translated error %q must never contain raw driver Detail/Message text", got)
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if sub == "" {
			continue
		}
		if len(s) >= len(sub) && indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestTranslateError_NotFound(t *testing.T) {
	got := translateError(nil)
	if got != nil {
		t.Errorf("translateError(nil) = %v, want nil", got)
	}
}
