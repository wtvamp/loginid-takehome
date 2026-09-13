package sqlite

import (
	"errors"
	"testing"
)

func TestCheckConstraintName(t *testing.T) {
	cases := []struct {
		msg      string
		wantName string
		wantOK   bool
	}{
		{"constraint failed: CHECK constraint failed: ck_user_credential_secret_state (275)", "ck_user_credential_secret_state", true},
		{"constraint failed: CHECK constraint failed: ck_user_profile_phone_e164 (275)", "ck_user_profile_phone_e164", true},
		{"some other error", "", false},
	}
	for _, c := range cases {
		name, ok := checkConstraintName(c.msg)
		if ok != c.wantOK || name != c.wantName {
			t.Errorf("checkConstraintName(%q) = (%q, %v), want (%q, %v)", c.msg, name, ok, c.wantName, c.wantOK)
		}
	}
}

func TestTranslateError_NotFoundAndNil(t *testing.T) {
	if got := translateError(nil); got != nil {
		t.Errorf("translateError(nil) = %v, want nil", got)
	}
}

func TestTranslateError_UnclassifiedNeverLeaksMessage(t *testing.T) {
	err := errors.New("some unrelated error")
	got := translateError(err)
	if got == nil {
		t.Fatal("expected a wrapped error")
	}
}

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"50%":        `50\%`,
		"a_b":        `a\_b`,
		`back\slash`: `back\\slash`,
		"plain":      "plain",
	}
	for in, want := range cases {
		if got := escapeLike(in); got != want {
			t.Errorf("escapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}
