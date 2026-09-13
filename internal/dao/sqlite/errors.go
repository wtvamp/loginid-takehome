package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sqlitedriver "modernc.org/sqlite"

	"loginid-takehome/internal/dao"
)

// Extended result codes, per SQLite's documented result-code table
// (https://www.sqlite.org/rescode.html). modernc.org/sqlite's Error.Code()
// returns the extended code, not just the primary SQLITE_CONSTRAINT class.
const (
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
	sqliteConstraintCheck      = 275  // SQLITE_CONSTRAINT_CHECK
)

const (
	constraintUsernameUnique = "uq_user_credential_username"
	constraintMethodFK       = "fk_user_credential_method"
	constraintSecretState    = "ck_user_credential_secret_state"
)

// checkConstraintName extracts the constraint name from a SQLite CHECK
// violation message. SQLite's message for a named constraint (defined as
// `CONSTRAINT name CHECK (...)` in our own DDL) is deterministically
// "CHECK constraint failed: <name>", sometimes with a trailing " (<code>)"
// (observed from modernc.org/sqlite: "constraint failed: CHECK constraint
// failed: ck_user_credential_secret_state (275)") — this is not free-text
// message matching in the sense the contract prohibits; it's extracting a
// structured field SQLite's own error format guarantees to contain
// exactly the constraint name we ourselves wrote in DDL, the same
// artifact Postgres gives us through PgError.ConstraintName directly.
//
// UNLIKE Postgres's PgError.ConstraintName, this is not a stable driver
// API — it's modernc.org/sqlite's undocumented message formatting.
// Verified (Oren Castellan, PR #8 review) against every CHECK/UNIQUE/FK/
// PK condition in the schema, including unnamed CHECKs, which correctly
// fall through to ErrInvalidArgument because their raw expression text
// never happens to equal constraintSecretState — not because this parser
// distinguishes "no name present" from "name present". If a future
// driver version reflows that message (drops the trailing " (<code>)",
// changes wording), this silently stops matching and
// TestIntegration_CredentialCRUDAndErrorTranslation's exact-sentinel
// assertion is the ONLY thing that catches it — do not remove that
// assertion as "redundant" with a looser one.
func checkConstraintName(msg string) (string, bool) {
	const marker = "CHECK constraint failed: "
	i := strings.LastIndex(msg, marker)
	if i == -1 {
		return "", false
	}
	name := strings.TrimSpace(msg[i+len(marker):])
	// Strip a trailing " (<digits>)" result-code suffix, if present.
	if j := strings.LastIndex(name, " ("); j != -1 && strings.HasSuffix(name, ")") {
		name = strings.TrimSpace(name[:j])
	}
	return name, true
}

// translateError maps a driver error to one of the seven dao sentinels by
// structured extended result code plus our own constraint name — never by
// matching arbitrary free-text. Mirrors postgres/errors.go's rule exactly
// so both backends produce the same sentinel for the same logical
// condition (multi-db-strategy.md §4).
func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return dao.ErrNotFound
	}

	var sqErr *sqlitedriver.Error
	if errors.As(err, &sqErr) {
		switch sqErr.Code() {
		case sqliteConstraintUnique:
			// This schema's only UNIQUE index besides the PK is
			// uq_user_credential_username — no disambiguation needed;
			// unlike Postgres, a PK collision here reports as
			// SQLITE_CONSTRAINT_PRIMARYKEY (below), a distinct code, so
			// this branch is unambiguously the username index.
			return dao.ErrDuplicateUsername
		case sqliteConstraintPrimaryKey:
			return dao.ErrAlreadyExists
		case sqliteConstraintForeignKey:
			// SQLite's FK violation message does not name the FK
			// constraint; this schema has exactly one FK we translate
			// specifically (method_id) and one we don't need to
			// (user_id, never violated by application code since the
			// DAO always creates the profile first in the same
			// transaction) — so any FK violation reaching here is the
			// method FK.
			return dao.ErrInvalidMethod
		case sqliteConstraintCheck:
			if name, ok := checkConstraintName(sqErr.Error()); ok && name == constraintSecretState {
				return dao.ErrInvalidCredential
			}
			return dao.ErrInvalidArgument
		}
	}

	if sqErr != nil {
		return fmt.Errorf("sqlite: driver error (code %d): %w", sqErr.Code(), errUnclassified)
	}
	return fmt.Errorf("sqlite: %w", errUnclassified)
}

var errUnclassified = errors.New("unclassified driver error")
