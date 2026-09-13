package postgres

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"loginid-takehome/internal/dao"
)

// Constraint names, pinned because they are our own DDL artifact, not the
// engine's (multi-db-strategy.md §4). Named identically in this package's
// schema and referenced here so translation never depends on table/column
// position, which can drift independently between backends.
const (
	constraintUsernameUnique = "uq_user_credential_username"
	constraintMethodFK       = "fk_user_credential_method"
	constraintSecretState    = "ck_user_credential_secret_state"
)

const (
	sqlstateUniqueViolation     = "23505"
	sqlstateForeignKeyViolation = "23503"
	sqlstateCheckViolation      = "23514"
)

// translateError maps a driver error to one of the seven dao sentinels by
// structured SQLSTATE code plus our own constraint name — never by
// matching free-text error messages (multi-db-strategy.md §4). Anything
// that isn't a recognized driver error is wrapped with %w and passed
// through. The raw driver error (including any Postgres DETAIL/Key(...)=
// (...) text) is deliberately never included in the sentinel path — only
// the structured code and constraint name are inspected, and neither is
// echoed into the returned sentinel, so a caller logging the sentinel
// never logs PII (decisions/error-semantics.md F50).
func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return dao.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case sqlstateUniqueViolation:
			if pgErr.ConstraintName == constraintUsernameUnique {
				return dao.ErrDuplicateUsername
			}
			// Any other unique violation (a primary-key collision) is the
			// internal-error-only sentinel — never caller-reachable, since
			// no caller supplies an ID (05's ruling on Oren's finding #1).
			return dao.ErrAlreadyExists
		case sqlstateForeignKeyViolation:
			if pgErr.ConstraintName == constraintMethodFK {
				return dao.ErrInvalidMethod
			}
			return dao.ErrInvalidArgument
		case sqlstateCheckViolation:
			if pgErr.ConstraintName == constraintSecretState {
				return dao.ErrInvalidCredential
			}
			return dao.ErrInvalidArgument
		}
	}

	// Not a recognized structured condition — wrap for logs, sanitized:
	// only the SQLSTATE code (if any) travels with it, never the raw
	// message/DETAIL text.
	if pgErr != nil {
		return fmt.Errorf("postgres: driver error (sqlstate %s): %w", pgErr.Code, errUnclassified)
	}
	return fmt.Errorf("postgres: %w", errUnclassified)
}

var errUnclassified = errors.New("unclassified driver error")
