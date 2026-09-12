package dao

import "errors"

// The seven sentinels from multi-db-strategy.md §4, verbatim. Every backend
// implementation (S3/S4) returns one of these for its matching condition —
// the service layer never distinguishes sql.ErrNoRows from a
// SQLite-specific miss. Anything not in this set is wrapped with %w and
// passed through. Test with errors.Is.
var (
	ErrNotFound = errors.New("dao: not found")

	// ErrAlreadyExists is reachable only via an actual UUID collision or a
	// duplicated retry on Create — never via caller input, since Create
	// never accepts a caller-supplied ID (a non-empty ID is
	// ErrInvalidArgument instead). It is an internal-error signal, not a
	// caller-facing contract: 05's conformance suite is explicitly NOT
	// required to exercise this sentinel, and no test should be built
	// around making it reachable from caller input (05's ruling on
	// Oren's original-review finding #1).
	ErrAlreadyExists = errors.New("dao: already exists")

	ErrDuplicateUsername = errors.New("dao: duplicate username")
	ErrInvalidMethod     = errors.New("dao: unknown or inactive auth method")
	ErrInvalidQuery      = errors.New("dao: invalid query")
	ErrInvalidArgument   = errors.New("dao: invalid argument")
	ErrInvalidCredential = errors.New("dao: credential violates secret-state rules")
)
