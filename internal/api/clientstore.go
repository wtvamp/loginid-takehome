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
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters used when HASHING A NEW secret — hashSecret's own
// cost profile, matching threat-model.md Asset 1's floor. verifySecret
// below deliberately does NOT use these constants to recompute a hash:
// it reads the cost parameters (m, t, p) out of the stored PHC string
// itself. A verifier that ignored the embedded parameters and always
// recomputed with these constants would make the PHC string's own
// self-description pointless — the day this profile changes, every
// existing hash would break at once with no way to tell an old row from
// a new one, exactly the failure oauth_client's schema (no hash_algo/
// hash_cost columns, unlike user_credential) was designed to avoid by
// relying on the hash string being self-describing (Priya Nandakumar's
// review, handoff-03-auth.md v7).
const (
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// phcVersion is Argon2id's own version identifier (argon2.Version,=19,
// i.e. 0x13) as it appears in a PHC string's "v=" field — a property of
// the algorithm itself, not a cost knob, so unlike time/memory/threads
// it is not expected to change independently of upgrading the algorithm
// version.
const phcVersion = argon2.Version

// hashSecret returns an Argon2id hash of secret encoded as a standard
// PHC string: $argon2id$v=19$m=<memory>,t=<time>,p=<threads>$<salt>$<hash>
// (RFC-adjacent PHC string format; RawStdEncoding, unpadded, matching
// the same encoding user_credential.secret already uses for its own
// Argon2id hashes — one encoding convention project-wide, not two).
// Self-describing: deploy/gen-argon2-hash.go emits the same shape, and
// verifySecret below reads the embedded parameters back rather than
// assuming they match argon2Time/argon2Memory/argon2Threads above.
func hashSecret(secret string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("api: generating salt: %w", err)
	}
	hash := argon2.IDKey([]byte(secret), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return encodePHC(argon2Memory, argon2Time, argon2Threads, salt, hash), nil
}

// encodePHC always stamps the CURRENT phcVersion — it's for producing a
// brand-new hash under this package's own live cost profile, never for
// re-serializing a parsed hash's own (possibly different, possibly
// older) parameters.
func encodePHC(memory, time, threads int, salt, hash []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		phcVersion, memory, time, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// parsedPHC is a PHC-string Argon2id hash's parameters, read off the
// string itself rather than assumed from this package's own constants —
// the whole reason to use PHC instead of the bare "<salt>$<hash>"
// encoding this replaced.
type parsedPHC struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	hash    []byte
}

// parsePHC parses a PHC-format Argon2id hash string
// ($argon2id$v=<ver>$m=<mem>,t=<time>,p=<threads>$<salt>$<hash>) into
// its components. Returns an error for anything that doesn't match this
// exact shape — verifySecret treats a parse failure as "does not
// verify," never as a reason to fall back to any other interpretation.
func parsePHC(encoded string) (parsedPHC, error) {
	// "$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>" split on "$" is:
	// ["", "argon2id", "v=19", "m=65536,t=1,p=4", "<salt>", "<hash>"]
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return parsedPHC{}, fmt.Errorf("api: not a recognized argon2id PHC string")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return parsedPHC{}, fmt.Errorf("api: parsing PHC version field %q: %w", parts[2], err)
	}
	if version != phcVersion {
		return parsedPHC{}, fmt.Errorf("api: PHC string names argon2 version %d, this package verifies version %d", version, phcVersion)
	}

	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return parsedPHC{}, fmt.Errorf("api: PHC parameter field %q does not have exactly 3 comma-separated parameters", parts[3])
	}
	var memory, timeCost, threads uint64
	var err error
	if memory, err = parsePHCParam(params[0], "m"); err != nil {
		return parsedPHC{}, err
	}
	if timeCost, err = parsePHCParam(params[1], "t"); err != nil {
		return parsedPHC{}, err
	}
	if threads, err = parsePHCParam(params[2], "p"); err != nil {
		return parsedPHC{}, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return parsedPHC{}, fmt.Errorf("api: decoding PHC salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return parsedPHC{}, fmt.Errorf("api: decoding PHC hash: %w", err)
	}

	return parsedPHC{
		memory:  uint32(memory),
		time:    uint32(timeCost),
		threads: uint8(threads),
		salt:    salt,
		hash:    hash,
	}, nil
}

// parsePHCParam parses one "<name>=<digits>" field (e.g. "m=65536") and
// requires the field's name to match wantName exactly, in the position
// PHC's own fixed m,t,p ordering requires — a defensive check against a
// hand-edited or malformed string silently reading the wrong value into
// the wrong slot.
func parsePHCParam(field, wantName string) (uint64, error) {
	name, digits, found := strings.Cut(field, "=")
	if !found || name != wantName {
		return 0, fmt.Errorf("api: PHC parameter field %q is not in the expected %q= form", field, wantName)
	}
	v, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("api: parsing PHC parameter %q: %w", field, err)
	}
	return v, nil
}

// verifySecret reports whether secret matches encodedHash, a PHC-format
// Argon2id hash produced by hashSecret (or by deploy/gen-argon2-hash.go,
// which emits the identical shape). Recomputes using the memory/time/
// threads parameters PARSED OUT OF encodedHash itself — never this
// package's own argon2Time/argon2Memory/argon2Threads constants — so a
// hash created under a different (older or newer) cost profile still
// verifies correctly; only the embedded parameters and the embedded
// salt/hash length (via keyLen = len(parsed hash)) participate.
// Comparison is constant-time (crypto/subtle) — Argon2id's own cost
// already dominates timing versus a plain []byte equality, but there is
// no reason to reintroduce a length/byte-position side channel on top
// of it for free.
func verifySecret(secret, encodedHash string) bool {
	parsed, err := parsePHC(encodedHash)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(secret), parsed.salt, parsed.time, parsed.memory, parsed.threads, uint32(len(parsed.hash)))
	return subtle.ConstantTimeCompare(got, parsed.hash) == 1
}

// needsRehash reports whether encodedHash's embedded cost parameters
// differ from this package's CURRENT profile (argon2Memory/argon2Time/
// argon2Threads) — the mechanism by which a cost-profile bump migrates
// existing rows forward opportunistically (verify-then-rehash-on-next-
// successful-auth, Marcus Ilori's ruling, handoff-03-auth.md v7) instead
// of requiring a live-credential migration. Only ever called after
// verifySecret has already succeeded against encodedHash — this reports
// "should be re-hashed under the current profile," not "is valid."
// encodedHash is assumed parseable here (verifySecret already parsed it
// successfully in the same call); a parse failure reports false rather
// than panicking, since "can't tell, so don't rehash" is the safe
// default for a function only ever called after a successful verify.
func needsRehash(encodedHash string) bool {
	parsed, err := parsePHC(encodedHash)
	if err != nil {
		return false
	}
	return parsed.memory != argon2Memory || parsed.time != argon2Time || parsed.threads != argon2Threads
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

	// UpdateSecretHash replaces clientID's stored hash — the
	// opportunistic-rehash mechanism (Marcus Ilori's ruling,
	// handoff-03-auth.md v7): called by tokenhandler.go after a
	// successful grant when needsRehash reports the stored hash's
	// embedded cost parameters have drifted from this package's current
	// profile, so an existing row migrates forward the next time its
	// owner successfully authenticates, without a live-credential
	// migration. Best-effort from the caller's perspective — a failure
	// here does not fail the grant already in progress, since the
	// credential the caller presented was already correctly verified.
	UpdateSecretHash(ctx context.Context, clientID, newHash string) error
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

// UpdateSecretHash writes newHash for clientID — only ever called on a
// row whose secret_state is already 'set' (a just-verified grant), so
// this never needs to touch secret_state itself; the migration's own
// BEFORE UPDATE trigger sets updated_at. Same clientStoreQueryTimeout
// bound as Get, for the same reason (Nolan Reyes's review): this must
// not block indefinitely if the issuer database goes away mid-request.
func (s *PostgresClientStore) UpdateSecretHash(ctx context.Context, clientID, newHash string) error {
	ctx, cancel := context.WithTimeout(ctx, clientStoreQueryTimeout)
	defer cancel()

	result, err := s.db.ExecContext(ctx, `
		UPDATE oauth_client SET client_secret_hash = $1 WHERE client_id = $2 AND secret_state = 'set'
	`, newHash, clientID)
	if err != nil {
		return fmt.Errorf("api: updating oauth_client secret hash: %w", err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		// Not an error: the row could have been disabled/revoked between
		// the successful Get+verify and this call (a narrow race,
		// harmless either way — a revoked row rehashing its old secret
		// would be pointless, and a genuinely absent row can't happen
		// since Get just found it moments before). Logged by the caller
		// (tokenhandler.go), not here — this package doesn't hold a
		// logger.
		return fmt.Errorf("api: oauth_client row %q no longer has secret_state='set'", clientID)
	}
	return nil
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
