package dao

import "regexp"

// uuidPattern matches ONLY the canonical 36-character lowercase hyphenated
// UUID form (8-4-4-4-12 lowercase hex). Deliberately not uuid.Parse or a
// case-insensitive pattern: those accept brace-wrapped, urn:uuid:-prefixed,
// unhyphenated, and uppercase-hex variants too, and Postgres normalizes
// all of them to the same row while SQLite's TEXT column byte-compares
// against whatever canonical form was actually stored — so a permissive
// validator doesn't close the cross-backend divergence, it relocates it
// from malformed ids (now rejected) to well-formed non-canonical ones
// (silently accepted, then treated differently per backend). Every write
// path must therefore also store exactly this canonical lowercase form.
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidateID is the single enforcement site for "is this a syntactically
// valid identifier" — called by every backend method taking an id,
// before any SQL runs, per 05's amendment A4 (revised after Yusuf's
// objection turn). A malformed OR empty id is ErrInvalidArgument (a
// caller bug), never ErrNotFound (a data outcome) — consistent with
// every other malformed-input case in this contract, and load-bearing
// for DeleteProfile specifically: this contract treats ErrNotFound from
// a delete as "already gone, treat as success", so a typo'd id on a
// subject-deletion request must never be allowed to read as ErrNotFound,
// or a GDPR erasure request with a typo reports completed while nothing
// was deleted.
//
// Validated call sites only (05's explicit list — not "every method"):
// Profiles().Get/Update/Upsert/Delete id; Credentials().Update/Delete id;
// Credentials().ListByUserID userID; Methods().Get id; DeleteProfile id.
// NOT validated: Create's ID (must be "", a different rule entirely —
// ValidateCreateID), CreateProfileWithCredential's c.UserID (ignored and
// overwritten), DeleteProfile's externalRef (opaque, not a UUID),
// GetByUsername/GetByName (not identifiers).
func ValidateID(id string) error {
	if !uuidPattern.MatchString(id) {
		return ErrInvalidArgument
	}
	return nil
}
