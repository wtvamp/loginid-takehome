// clientstore.go implements the token-issuer's client store per
// handoff-03-auth.md v6/LT-51: client_id, name, an Argon2id-hashed
// secret (same floor as threat-model.md Asset 1's password hashing),
// granted scope, audience, and status.
//
// Backed by a real, persistent PostgreSQL database — a separate
// "issuer" database (migrations/issuer/00001_initial_schema.sql), not
// an in-process map — per Priya Nandakumar's (05) explicit ruling on
// this story: unlike every other in-process component this track has
// built (RateLimiter, TouchCounter, GrantLimiter), a client credential
// has to survive a pod restart, and the QA client seeded for the PO's
// live-URL review has to be seeded out-of-band (04's runbook), never by
// this application's own code or a migration — "migrations carry
// reference data, never credentials." Lives in its own database, not
// 05's main DAO schema, so a cross-database join to user_profile is
// structurally impossible, not just policy-forbidden (F8 resolution,
// handoff-03-auth.md v3).
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters — one fixed cost profile for every client secret
// (and the fixed dummy hash timing-parity depends on, below). Matches
// threat-home.md Asset 1's own floor: this is not a separate, weaker
// hashing decision for machine credentials, it's the same discipline
// applied to a second credential class.
const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// hashSecret returns an Argon2id hash of secret, encoded as
// "<base64-salt>$<base64-hash>" — self-contained, no separate parameter
// storage needed since every hash in this store uses the same fixed
// profile above.
func hashSecret(secret string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("api: generating salt: %w", err)
	}
	hash := argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash), nil
}

// verifySecret reports whether secret matches encodedHash, produced by
// hashSecret. Comparison is constant-time (crypto/subtle) — Argon2id's
// own cost already dominates timing versus a plain []byte equality, but
// there is no reason to reintroduce a length/byte-position side channel
// on top of it for free.
func verifySecret(secret, encodedHash string) bool {
	parts := strings.SplitN(encodedHash, "$", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash is computed once, at package init, from a fixed (not
// secret — its value doesn't matter, only that it's fixed and uses the
// SAME cost parameters as every real hash) salt and secret. The
// unknown-client_id path in the token handler runs verifySecret against
// this exact value before returning invalid_client, so an unknown
// client_id and a wrong-secret-for-a-known-client_id take
// statistically indistinguishable time (Tobias Lindqvist's objection,
// LT-51 refinement — RFC 6749 §5.2's identical-response-body
// requirement closes the content side channel; this closes the timing
// one, which the body requirement alone leaves wide open).
var dummyHash = mustHashSecret("dummy-fixed-secret-never-issued-to-any-real-client")

func mustHashSecret(secret string) string {
	h, err := hashSecret(secret)
	if err != nil {
		// Only reachable if crypto/rand itself fails at process start —
		// a condition this process cannot usefully continue past anyway
		// (every real hashSecret call would fail identically).
		panic(fmt.Sprintf("api: computing fixed dummy Argon2id hash: %v", err))
	}
	return h
}

// ClientRecord is one row in the client store.
type ClientRecord struct {
	ClientID         string
	Name             string // accountability — no shared credentials across a team
	ClientSecretHash string // Argon2id, hashSecret's encoding
	GrantedScope     Scope  // exactly one — see ClientStore's doc comment
	Audience         string // the JWT aud this client's tokens are minted for
	Active           bool
}

// ClientStore is the token-issuer's client datastore — client
// authentication and scope/audience lookup, keyed by client_id.
//
// GrantedScope is a single Scope, not a set, even though
// handoff-03-auth.md v6 describes "granted_scopes" as a provisioned
// subset of the four possible values: this project's own provisioning
// model (decisions/search-authz-scoping.md; LT-40's singleScope) commits
// to exactly one scope per credential — "a client is provisioned for the
// one use case its credential exists for" — and AuthContext (LT-39)
// carries exactly one Scope per token by the same design. A client
// record holding more than one granted scope would have no way to
// produce a token LT-40's verifier accepts (which rejects a multi-scope
// token outright, deliberately, not an oversight this story should route
// around). So provisioning is one client credential per scope a caller
// needs, not one client holding several — consistent with, not a
// deviation from, the system's existing single-scope-per-token
// commitment.
type ClientStore interface {
	// Get returns the record for clientID, ok=false if no such client
	// exists at all (a different outcome from an inactive client, which
	// Get still returns — the token handler decides what "not usable"
	// means, this interface only reports what's on file).
	Get(ctx context.Context, clientID string) (rec ClientRecord, ok bool, err error)
}

// clientStoreQueryTimeout bounds every Get call against the issuer
// database independently of whatever's left on the caller's context —
// Nolan Reyes's review of this story: an issuer database that goes away
// mid-process-lifetime (not just "absent at startup", which is the
// case NewIssuerRouter's nil-store fail-closed path already covers)
// must not leave /auth/token requests blocked indefinitely on a dial or
// query against a dead connection. This is the query-side half of that
// fix; NewPostgresClientStore's *sql.DB pool bounds (set by
// cmd/api-service/main.go) are the other half.
const clientStoreQueryTimeout = 3 * time.Second

// PostgresClientStore is a real, persistent ClientStore backed by the
// issuer database's oauth_client table (migrations/issuer/
// 00001_initial_schema.sql). Constructed once at startup from a *sql.DB
// dialed against config.Config.IssuerDBDSN — always the "pgx" driver,
// since the issuer database has no SQLite/CockroachDB peer to abstract
// over (unlike 05's main DAO schema), so this reaches for
// database/sql directly rather than routing through dao.New's
// multi-backend factory, which was never asked to know about this
// table.
type PostgresClientStore struct {
	db *sql.DB
}

// NewPostgresClientStore wraps an already-opened *sql.DB. Opening (and
// its lifetime) is cmd/api-service/main.go's responsibility, same
// pattern as LT-52's separate /healthz ping handle — this type never
// calls sql.Open itself.
func NewPostgresClientStore(db *sql.DB) *PostgresClientStore {
	return &PostgresClientStore{db: db}
}

// Get deliberately does not distinguish "no such client_id",
// "provisioned but secret_state != 'set'", or "status = 'disabled'" in
// its ok/err results — all three come back as ok=true with whatever's
// on file (or ok=false only for a genuinely absent row), and it's
// tokenhandler.go's job to decide what counts as usable. This keeps
// every one of those states on the same code path through
// verifySecret/dummyHash, so none of them becomes a second timing
// signal alongside the unknown-client_id one dummyHash already closes.
//
// This method's own internal work is NOT perfectly symmetric between a
// found and not-found row (a found row does a full Scan plus
// parsePostgresTextArray; sql.ErrNoRows returns immediately) — negligible
// today only by construction, since Argon2id's ~64 MiB cost in
// verifySecret dwarfs a single indexed lookup and parsing a few bytes
// (Oren Castellan, PR #34 review). If Get ever grows real additional
// work on the found path (e.g. a join), re-examine whether that gap is
// still negligible before assuming the timing-parity invariant still
// holds on tokenhandler.go's say-so alone.
func (s *PostgresClientStore) Get(ctx context.Context, clientID string) (ClientRecord, bool, error) {
	var rec ClientRecord
	var secretHash sql.NullString
	var scopesRaw string
	var status string

	ctx, cancel := context.WithTimeout(ctx, clientStoreQueryTimeout)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT client_id, name, client_secret_hash, granted_scopes, audience, status
		FROM oauth_client
		WHERE client_id = $1
	`, clientID)
	err := row.Scan(&rec.ClientID, &rec.Name, &secretHash, &scopesRaw, &rec.Audience, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ClientRecord{}, false, nil
	}
	if err != nil {
		return ClientRecord{}, false, fmt.Errorf("api: querying oauth_client: %w", err)
	}
	scopes, err := parsePostgresTextArray(scopesRaw)
	if err != nil {
		return ClientRecord{}, false, fmt.Errorf("api: parsing granted_scopes for %q: %w", clientID, err)
	}
	if len(scopes) != 1 {
		// The migration's own CHECK (ck_oauth_client_granted_scopes_single)
		// makes this unreachable in practice — surfaced as an error rather
		// than silently taking scopes[0] of an empty slice, in case that
		// constraint is ever relaxed without this code being revisited.
		return ClientRecord{}, false, fmt.Errorf("api: oauth_client row %q has %d granted scopes, want exactly 1", clientID, len(scopes))
	}

	rec.ClientSecretHash = secretHash.String // "" when NULL (secret_state != "set")
	rec.GrantedScope = Scope(scopes[0])
	rec.Active = status == "active"
	return rec, true, nil
}

// parsePostgresTextArray decodes Postgres's default text output format
// for a TEXT[] column (e.g. "{profile:search}") into its elements. This
// project's own scope values are plain identifiers — letters, digits,
// ":" — never containing a comma, brace, backslash, or double quote, so
// this deliberately does not implement the general Postgres array
// literal grammar (quoted elements, backslash escaping, NULL elements):
// pgx/v5's stdlib driver has no built-in database/sql Scanner for array
// types, and a hand-rolled general parser would be untested complexity
// this table's actual value domain never exercises. Any input outside
// that domain is an error, not a best-effort guess.
func parsePostgresTextArray(raw string) ([]string, error) {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return nil, fmt.Errorf("not a Postgres array literal: %q", raw)
	}
	inner := raw[1 : len(raw)-1]
	if inner == "" {
		return nil, nil
	}
	elems := strings.Split(inner, ",")
	for _, e := range elems {
		if e == "" || strings.ContainsAny(e, `{}"\`) {
			return nil, fmt.Errorf("unsupported array element %q in %q — only plain unquoted values are supported", e, raw)
		}
	}
	return elems, nil
}
