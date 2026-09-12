# Multi-Database Abstraction Strategy

Owner: 05-data-ops. Implemented by: `03-engineering-delivery`, from the prose contract below — this project is still in planning phase, so the interface and schema exist here as design, not as `.go`/`.sql` files, until there's an explicit go-ahead to write code. Full research trail: `research-data-ops-best-practices.md`.

## The interface contract (prose, not code)

Three repository interfaces, one per entity, each implemented by two backend packages (below) and never containing SQL themselves — the service layer depends only on these shapes:

- **`ProfileRepository`** — `Get(ctx, id) (*UserProfile, error)`; `Search(ctx, query) (results []UserProfile, total int, error)` where `query` filters by name (partial/fuzzy), phone (exact/prefix), region, country, plus limit/offset for pagination; `Create(ctx, *UserProfile) (*UserProfile, error)`; `Update(ctx, *UserProfile) (*UserProfile, error)` implemented as a portable upsert; `Delete(ctx, id) error`.
- **`CredentialRepository`** — `GetByUsername(ctx, username) (*UserCredential, error)` as the primary auth lookup path; `ListByUserID(ctx, userID) ([]UserCredential, error)`; `Create`, `Update`, `Delete` mirroring `ProfileRepository`'s shape. Never accepts or returns `UserProfile` fields.
- **`AuthMethodRepository`** — `Get(ctx, id)`, `GetByName(ctx, name)`, `List(ctx, activeOnly bool)`. Read-heavy; writes to this table are an operational action (adding a supported method), not part of the normal request path.

Domain types implied by the above: `UserProfile` (`ID`, `Name`, `Phone`, `StreetAddress`, `Locality`, `Region`, `PostalCode`, `Country`, `Source` — `"direct"` or `"idp_cache"` — `CreatedAt`, `UpdatedAt`); `UserCredential` (`ID`, `UserID` FK, `Username` unique, `MethodID` FK, `Secret` as bytes/nil, `HashAlgo`, `HashCost`, `CreatedAt`, `UpdatedAt`); `AuthMethod` (`ID`, `Name`, `RequiresSecret` bool, `IsActive` bool). A shared `ErrNotFound` sentinel is returned by every backend implementation for a missing row, so the service layer never has to distinguish `sql.ErrNoRows` from a SQLite-specific miss.

Two concrete implementations satisfy all three interfaces:

- `postgres` package — serves both PostgreSQL and CockroachDB from one implementation. They share enough of the wire protocol and SQL surface (see the compatibility table below) that a second implementation would be duplicated effort, not genuine portability work.
- `sqlite` package — a separate implementation. SQLite diverges enough (no native UUID/BOOLEAN, no trigram index, app-side ID generation) that forcing it through the Postgres implementation would mean littering that code with backend conditionals, which is the exact failure mode a repository-per-backend split is meant to avoid.

The service layer depends only on the interfaces and is constructed with whichever concrete implementation the startup config names. Backend choice is a wiring decision, never a runtime branch inside business logic.

## Why not one implementation with dialect branches

A single implementation with `if backend == "sqlite"` scattered through query construction was considered and rejected: it couples unrelated dialect concerns into one code path, makes it easy to add a Postgres-only feature (trigram search) without noticing SQLite silently falls back to nothing, and is harder to test in isolation per backend. Two implementations behind one interface is the standard Go shape for this problem (see sources in the research doc) and is what the interface contract above is designed against.

## Dialect differences that survive into the DAO layer

| Concern | Postgres / CockroachDB | SQLite | Handling |
|---|---|---|---|
| Primary key | `UUID DEFAULT gen_random_uuid()`, server-generated | `TEXT`, app-generated before insert | DAO layer generates the UUID in Go for SQLite; server-generates for Postgres/CockroachDB. Callers never see the difference — `Create()` returns the populated `ID` either way. |
| Placeholders | `$1, $2, ...` | `?` | Handled by the driver/query builder, not hand-rolled per query. |
| Upsert | `INSERT ... ON CONFLICT (id) DO UPDATE SET ...` | Same syntax, supported since SQLite 3.24 | One query shape used everywhere. CockroachDB's `UPSERT` shorthand is deliberately not used, to avoid a second query dialect. |
| Fuzzy name search | `pg_trgm` GIN index, native on both Postgres and CockroachDB | No native equivalent | `Search()` behaves identically from the caller's side; the SQLite implementation falls back to a `LIKE '%term%'` scan. This is a documented, deliberate parity gap, not an oversight — SQLite is the local/dev/demo backend, not a production peer. |
| JSON | `JSONB`, indexable, has query operators | `TEXT`/`BLOB` with a differently-formatted internal `JSONB` (SQLite 3.45+) that is not disk-compatible with Postgres's | Not currently used in the reference schema. If a raw IDP `/identity` payload is ever cached, it is stored as opaque `TEXT` at the DAO boundary — no JSON-operator filtering pushed into either implementation, so the column stays portable. |
| Boolean | Native `BOOLEAN` | `INTEGER` 0/1 | Mapped at the DAO boundary; Go `bool` on both sides of the interface. |

## Schema shape (prose, not DDL)

`auth_method` — lookup table, not a native enum: `id` (UUID), `name` (unique — `"password"`, `"passkey"`, `"otp"`, `"oidc"`, ...), `requires_secret` (bool, false for passkey/WebAuthn-style methods), `is_active` (bool), `created_at`. Seeded with one `"password"` row; new methods are inserted as data, never a migration.

`user_profile` — pure PII, no secrets: `id` (UUID PK), `name`, `phone` (normalized E.164 at write time), `street_address`, `locality`, `region`, `postal_code`, `country` (ISO 3166-1 alpha-2), `source` (`"direct"` or `"idp_cache"`, constrained), `created_at`, `updated_at`.

`user_credential` — this system's own login credentials, no PII: `id` (UUID PK), `user_id` (FK → `user_profile.id`, cascade delete), `username` (unique), `method_id` (FK → `auth_method.id`), `secret` (nullable, salted hash only, never plaintext), `hash_algo`, `hash_cost`, `created_at`, `updated_at`.

Indexes: B-Tree on `user_profile.phone`; trigram GIN on `user_profile.name` (Postgres/CockroachDB only); unique on `user_credential.username`; FK indexes on `user_credential.user_id` and `.method_id`.

Primary keys are UUID everywhere — server-generated (`gen_random_uuid()`) on Postgres/CockroachDB, app-generated in Go before insert on SQLite (which has no native UUID type). One PK strategy across backends, no per-backend branch in the DAO.

## Migration approach

- One shared SQL migration series covers the base schema (`CREATE TABLE`, standard column types, the portable upsert-compatible constraints) since it's close enough across all three backends for this schema's scope.
- A small, explicitly-named set of per-backend override files covers what genuinely can't be shared — currently exactly one: the `pg_trgm` GIN index on `user_profile.name`, which has no SQLite equivalent and is simply omitted there rather than faked.
- Recommended tool: **goose** — plain-SQL-first, minimal dependencies, and its style fits "mostly shared, occasionally per-backend" files more naturally than golang-migrate's stricter up/down pairing. golang-migrate is an equally defensible choice if the engineering track already has tooling preferences; Atlas is worth naming as the more sophisticated schema-as-state alternative but is more than this take-home's scope needs.
- Migration files live under version control alongside the DAO code (`03-engineering-delivery` owns the pipeline that runs them — see `../04-infra-devops/CLAUDE.md` for where/how they're actually executed in CI/CD).

## What the engineering track should take from this

1. Implement `ProfileRepository`, `CredentialRepository`, `AuthMethodRepository` per the interface contract above, once there's a go-ahead to write code — don't redesign the contract, flag back to this track if it doesn't fit.
2. Two packages: `postgres` (serves Postgres + CockroachDB) and `sqlite`. No third implementation needed unless a fourth backend is added to the assignment.
3. Use the schema shape above as the starting migration content for both backend families.
4. Never let `UserCredential` and `UserProfile` fields mix in either implementation, even as an internal convenience struct — that boundary is a governance control (see `pii-governance.md`), not just a modeling choice.
