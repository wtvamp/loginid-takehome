# Data Ops Research: Multi-Database DAO for `user_profile` / `user_credential`

**AI tooling note:** This research was produced with Claude Code (Sonnet 5), using the built-in `WebSearch` tool to gather current (2025–2026) sources, synthesized directly into this document. No code, DDL, or migration files were generated — this is a design-reasoning artifact only, per this track's scope in `../05-data-ops/CLAUDE.md`.

---

## 1. Schema design: splitting `user_profile` from `user_credential`

The assignment already names two separate entities, and that split matches common practice rather than being an arbitrary requirement.

**Why separate tables:** The standard pattern is a profile/identity table (name, address, phone — pure PII, no secrets) and a distinct credentials/logins table keyed by a foreign key to the profile. Reasons cited across sources:

- It avoids forcing NULL-able credential columns onto every profile row for users/contexts that don't have login credentials (e.g., a profile created via an IDP sync before a login is ever set up).
- The credentials table is kept "thin" — few columns, frequently read for auth checks — which helps indexing and cache locality, versus a wide profile table with many rarely-queried demographic columns (source: DoneDone user database model article).
- It supports one profile having *multiple* credentials (multiple login methods, multiple linked IDP accounts) — a 1:N relationship, not 1:1.
- For stronger compliance posture, some architectures go further and separate PII into a **physically distinct database/schema** with its own access controls, which is more convincing under a GDPR/CCPA audit than table-level separation alone (source: Authrim PII Separation Architecture). For a take-home, table-level separation with clear access-boundary documentation is the right depth — physical DB separation is worth a one-line "future hardening" note, not the actual design.

**Column choices, in line with what's asked:**

`user_profile`
- `id` (primary key — see §2 for type choice)
- `name` (or split `first_name`/`last_name` if search-by-name matters — see §4)
- `phone`
- `street_address`, `locality`, `region`, `postal_code`, `country` (matches the exact address shape the `/identity` endpoint in question 3 returns — worth mirroring so the IDP connector's response maps directly onto this table without a translation layer)
- `created_at`, `updated_at`
- optional: `source` / `origin` flag distinguishing profile rows created directly vs. hydrated from an IDP connector, which matters for the retention discussion in §5

`user_credential`
- `id` (primary key)
- `user_id` (FK → `user_profile.id`)
- `username`
- `method` (see below — the auth-method-extensibility column)
- `secret` (salted hash, never plaintext — see NIST 800-63B note below; naming it `secret` rather than `password` in the column itself keeps the schema honest as more methods are added)
- `created_at`, `updated_at`, and ideally a hash-algorithm/cost-factor reference column (NIST 800-63B: "a reference to the password hashing scheme used, including the cost factor, should be stored for each password to allow migration to new algorithms and work factors" — https://pages.nist.gov/800-63-4/sp800-63b.html)

**Password storage:** NIST SP 800-63B requires passwords be salted and hashed with an approved memory/CPU-hard scheme (PBKDF2, bcrypt, or Argon2), with the salt (≥32 bits) stored alongside the hash, and the cost factor stored so it can be increased over time as compute gets cheaper (https://pages.nist.gov/800-63-4/sp800-63b.html, https://netwrix.com/en/resources/blog/nist-password-guidelines/). This is `02-ai-security-architecture`'s call to make concretely, but the schema needs the columns to support it (a `secret` blob/text column plus metadata, not just a bare `password varchar`).

**Modeling `method` for extensibility:** The assignment explicitly wants room for auth methods beyond password. Two established patterns, in ascending order of flexibility:

1. **Enum/constrained string column** (`method` as a `CHECK`-constrained string or DB enum type: `'password'`, `'passkey'`, `'otp'`, `'oidc'`, …). Adding a new method is a value, not a schema change, as long as the column isn't a hard native DB enum type (Postgres native `ENUM` types *do* need `ALTER TYPE ... ADD VALUE` to extend — a `CHECK` constraint or plain `varchar` with app-level validation avoids that migration entirely, and is the more portable choice across SQLite, which has no native enum type at all).
2. **Lookup table** (`auth_method` table: `id`, `name`, `is_active`, `requires_secret`, …), referenced by `user_credential.method_id`. This is the pattern Moodle uses for its own multi-auth-method design (`prefix_auth_type` table with `id`, `name`, `isinternal`, `multiple`, `candisable`, `removable` — https://docs.moodle.org/dev/Multi_authentication) and is the more "textbook" extensible design: new methods are just new rows, and method-specific behavior flags (e.g., "this method never has a stored secret," relevant for passkeys/WebAuthn where the secret lives client-side) live on the lookup row instead of being encoded in application logic.

For a take-home, recommend the **lookup-table pattern** as the primary answer (it directly demonstrates "no schema migration needed to add a method") with the constrained-string enum mentioned as the lighter-weight alternative if simplicity is prioritized over the lookup table's normalization overhead. Passkey/WebAuthn credential storage (public key, credential ID, sign counter — see https://www.corbado.com/blog/passkey-webauthn-database-guide) is the concrete example of "a future auth method" worth citing: it needs materially different columns than a password (no secret hash, but a public key and counter), which is exactly why `method` should gate *optional/nullable* method-specific columns (or a side table keyed by `method`) rather than assuming every credential row shares the same secret shape.

**PII/credential non-conflation:** `user_credential` should never carry name/address/phone, and `user_profile` should never carry a password hash or raw secret — enforced structurally by having them be genuinely separate tables/FKs, not just a convention.

Sources:
- https://www.donedone.com/blog/building-the-optimal-user-database-model-for-your-application
- https://authrim.com/features/pii-separation/
- https://docs.moodle.org/dev/Multi_authentication
- https://www.corbado.com/blog/passkey-webauthn-database-guide
- https://pages.nist.gov/800-63-4/sp800-63b.html

---

## 2. Multi-database abstraction strategy (PostgreSQL / CockroachDB / SQLite)

**Pattern: repository interface, one implementation per driver.** This is the standard Go approach and matches what the assignment implies:

- Define a `ProfileRepository` / `CredentialRepository` interface at the domain/service layer (`Get`, `Search`, `Create`, `Update`, `Delete` — signatures in Go types, no SQL in the interface).
- Each backend (Postgres/CockroachDB — which can typically *share* one implementation given CockroachDB's wire-protocol and much of its SQL compatibility with Postgres — and SQLite) gets its own concrete struct implementing that interface, isolated in its own package, selected at startup via config/driver name.
- The service layer only ever depends on the interface; swapping backends is "import a different driver, construct a different concrete repository," per the threedots.tech and pawelgrzybek.com writeups on this exact pattern in Go (https://threedots.tech/post/repository-pattern-in-go/, https://pawelgrzybek.com/repository-pattern-in-go-service/). `database/sql` plus per-driver SQL string sets (or a query builder) is enough — no ORM is required, though one (e.g. sqlc, squirrel, GORM) is a reasonable implementation choice for the engineering track to justify.
- A "generic DAO over `database/sql`" library like `godal` (https://github.com/btnguyen2k/godal) demonstrates the pattern already has ready-made prior art across Postgres/SQLite/MySQL/etc., reinforcing that this is a solved, idiomatic shape rather than something novel to invent.

**Dialect differences that actually matter for this schema:**

| Concern | PostgreSQL | CockroachDB | SQLite |
|---|---|---|---|
| Primary key type | `SERIAL`/`BIGSERIAL` or `UUID` both fine | **UUID strongly preferred** — sequential/serial keys create write hotspots on a single range/node in a distributed cluster; `SERIAL` is offered only for Postgres-compatibility, and CockroachDB's own docs and engineers recommend `gen_random_uuid()` for real workloads (https://www.cockroachlabs.com/blog/how-to-choose-a-primary-key/, https://adhdecode.com/articles/cockroachdb/cockroachdb-uuid-vs-serial-primary-key/, https://github.com/cockroachdb/cockroach/issues/41258) | No native autoincrement concept the same way; `INTEGER PRIMARY KEY` is a rowid alias, or generate UUIDs at the app layer |
| **Decision for this schema:** use `UUID` as the primary key type universally across all three backends. It's the one choice that is simultaneously correct/required for CockroachDB, fully supported in Postgres, and easy to generate app-side for SQLite — avoiding a per-backend PK strategy branch in the DAO. |
| JSON columns | `JSONB` — binary, decomposed, indexable with GIN, O(1)-ish key lookup | Supports `JSONB` with Postgres-compatible semantics | JSON stored as `TEXT`/`BLOB` with JSON1 functions (`json_extract` etc.); SQLite's own `JSONB` (3.45+) is a *different, non-portable* binary format despite the shared name — not compatible with Postgres's `JSONB` on disk (https://sqlite.org/json1.html, https://fedoramagazine.org/json-and-jsonb-support-in-sqlite-3-45-0/) |
| **Implication:** don't rely on `JSONB` for anything the DAO needs to be portable — if a JSON blob column is used at all (e.g., raw IDP `/identity` payload cache), treat it as opaque text at the abstraction boundary and don't push filtering logic into JSON operators, since those operators are not portable across all three. |
| Upsert syntax | `INSERT ... ON CONFLICT (...) DO UPDATE SET ...` | Supports the same `ON CONFLICT` syntax (Postgres-compatible) *and* its own `UPSERT` shorthand, which skips the read that `ON CONFLICT DO UPDATE` performs and is faster when there's no secondary index involved (https://docs.cockroachlabs.com/docs/stable/upsert, https://www.cockroachlabs.com/blog/sql-upsert/) | Supports `INSERT ... ON CONFLICT (...) DO UPDATE SET ...` since SQLite 3.24 — same syntax family |
| **Implication:** `ON CONFLICT (...) DO UPDATE` is the one upsert syntax all three understand — use it as the portable default in the DAO's SQL, rather than CockroachDB's `UPSERT` shorthand, to keep one query string (modulo placeholder syntax) working across backends. |
| Placeholder syntax | `$1, $2, ...` | `$1, $2, ...` (Postgres-compatible) | `?` positional | This is the one genuinely mechanical per-driver difference the DAO layer has to account for — usually handled by the SQL builder/driver rather than hand-written strings. |
| General caveat | — | Despite wire-protocol and much syntax compatibility with Postgres, CockroachDB is **not a drop-in Postgres replacement**: distributed transaction semantics, lack of some Postgres extensions, and performance characteristics (e.g., the PK hotspot issue above) differ meaningfully. The official compatibility page is the authoritative diff (https://www.cockroachlabs.com/docs/stable/postgresql-compatibility). | SQLite has no concurrent-writer story, no network protocol, no `ROLE`/`GRANT` model — it's viable for local/dev/test but the DAO's SQLite implementation is realistically a "does it work for local dev and the take-home demo" backend, not a production peer of the other two. Worth stating explicitly rather than pretending it's an equal third option. |

Sources:
- https://www.cockroachlabs.com/blog/how-to-choose-a-primary-key/
- https://github.com/cockroachdb/cockroach/issues/41258
- https://adhdecode.com/articles/cockroachdb/cockroachdb-uuid-vs-serial-primary-key/
- https://docs.cockroachlabs.com/docs/stable/upsert
- https://www.cockroachlabs.com/blog/sql-upsert/
- https://www.cockroachlabs.com/docs/stable/postgresql-compatibility
- https://sqlite.org/json1.html
- https://fedoramagazine.org/json-and-jsonb-support-in-sqlite-3-45-0/
- https://threedots.tech/post/repository-pattern-in-go/
- https://pawelgrzybek.com/repository-pattern-in-go-service/
- https://github.com/btnguyen2k/godal

---

## 3. Migration tooling

Three commonly used Go-ecosystem tools, per current (2025–2026) usage:

- **golang-migrate** — the most established/widely used; plain up/down `.sql` files per version, supports many backends including Postgres and SQLite (CockroachDB via its Postgres driver, given wire compatibility). Simple, direct, no schema-diffing intelligence.
- **goose** — similarly lightweight, SQL- or Go-function-based migrations, minimal dependencies, broad backend support (Postgres, MySQL, SQLite, and more) (https://dev.to/shrsv/best-database-migration-tools-for-golang-ajf).
- **Atlas** — "Terraform for databases": you declare desired schema state and Atlas computes/applies the diff. It supports transactional migrations with locking to guarantee only one migration runs at a time, and has an integrity file to prevent migration-history conflicts across branches — properties golang-migrate/goose don't provide on their own (https://atlasgo.io/blog/2022/12/01/picking-database-migration-tool). Some teams combine Atlas (schema diffing) with goose (applying) rather than picking one exclusively (https://volomn.com/blog/database-migration-using-atlas-and-goose).

**Structuring migrations across three backends:** Given the schema is intentionally kept portable (UUID PKs everywhere, `ON CONFLICT` upsert syntax common to all three, JSON treated as opaque text), the great majority of DDL can be **one shared migration file set**, since standard `CREATE TABLE`/column-type SQL (aside from minor type-name differences) is close enough between Postgres, CockroachDB, and SQLite for this schema's scope. Recommend:
- One shared `.sql` migration series as the default, using types common to all three (`UUID` as text/native depending on driver support, plain `VARCHAR`/`TEXT`, no Postgres-only extensions like `JSONB` operators or `CITEXT` in the base schema).
- A small number of **per-backend override files** only where genuinely necessary — e.g., a Postgres/CockroachDB-only migration adding a `pg_trgm`/trigram GIN index for name search (§4), which has no SQLite equivalent and would need a different SQLite index strategy or to be skipped there.
- **golang-migrate or goose** as the recommended tool for this project (either is defensible; goose's plain-SQL-first style is a slightly easier fit for "mostly shared, occasionally per-backend" files). Atlas is worth naming as the more sophisticated alternative if the interviewer wants to see awareness of schema-as-state tooling, but isn't necessary to justify for a take-home of this size.

Sources:
- https://atlasgo.io/blog/2022/12/01/picking-database-migration-tool
- https://dev.to/shrsv/best-database-migration-tools-for-golang-ajf
- https://volomn.com/blog/database-migration-using-atlas-and-goose

---

## 4. Indexing for "search and retrieve" (question 2's REST API)

The API (question 2) needs to search/retrieve `user_profile` by at least name and phone, per the assignment. Indexing implications:

- **Phone lookup:** phone numbers are typically searched as exact or prefix matches, not fuzzy — a standard B-Tree index on the `phone` column is sufficient. Normalize phone format at write time (e.g., E.164) so the index is actually usable for equality lookups rather than fragmented by inconsistent formatting.
- **Name lookup:** name search is usually partial/fuzzy ("contains" or "starts with"), which a plain B-Tree index doesn't serve well for `LIKE '%term%'` queries. PostgreSQL's `pg_trgm` extension builds trigram-based GIN indexes that make arbitrary substring/fuzzy `LIKE` and similarity searches fast, and this is the standard PostgreSQL pattern for name search (https://medium.com/@saritasa/how-to-optimize-name-search-in-postgresql-with-trigram-and-btree-indexes-02c57eb27687). **CockroachDB also supports trigram indexes natively** (https://docs.cockroachlabs.com/docs/stable/trigram-indexes, https://www.cockroachlabs.com/blog/use-cases-trigram-indexes/), so this index strategy is portable between the two production-grade backends. SQLite has no native trigram/GIN equivalent — for the SQLite backend, either accept a full-scan `LIKE` for name search (acceptable for a local/dev/demo backend, consistent with §2's framing of SQLite as non-production-parity) or use SQLite's FTS5 extension for a rough parity if search-quality parity is important.
- **Case-insensitivity:** if usernames or names need case-insensitive equality (not fuzzy) matching, `CITEXT` (Postgres/CockroachDB) paired with a plain B-Tree index is the efficient pattern for that specific case — separate from the trigram index used for fuzzy/partial name search (https://medium.com/codex/case-insensitive-text-search-in-postgresql-whats-fast-and-what-fails-f836024c4590).
- **Combined/composite indexes:** if the API commonly filters by more than one field together (e.g., name + region), a composite index on that pair beats two separate single-column indexes for that specific query shape; keep single-column indexes only for fields queried independently.
- **`user_credential.username`:** should carry a unique index regardless of search requirements, since it's an auth lookup key, not just a profile search field.

Recommended index list for the schema:
- `user_profile`: B-Tree on `phone`; trigram GIN on `name` (Postgres/CockroachDB only — SQLite falls back to full scan or FTS5)
- `user_credential`: unique index on `username`; index on `user_id` (FK lookups); index on `method` (or `method_id` if using the lookup-table pattern) if the API/service ever filters credentials by method

Sources:
- https://medium.com/@saritasa/how-to-optimize-name-search-in-postgresql-with-trigram-and-btree-indexes-02c57eb27687
- https://docs.cockroachlabs.com/docs/stable/trigram-indexes
- https://www.cockroachlabs.com/blog/use-cases-trigram-indexes/
- https://medium.com/codex/case-insensitive-text-search-in-postgresql-whats-fast-and-what-fails-f836024c4590

---

## 5. PII data governance

Two distinct concerns: governing `user_profile` PII in general, and governing PII **cached** from the third-party IDP connector's `/identity` response (question 3).

**Data minimization (GDPR Art. 5(1)(c), NIST Privacy Framework CT.DM):** collect and retain only the PII fields actually needed for the stated purpose; this schema already matches the assignment's minimal field set (name, phone, the specific address subfields the `/identity` endpoint returns) rather than expanding it speculatively (https://www.strac.io/blog/data-minimization, https://www.strac.io/blog/nist-privacy-framework-data-minimization).

**Storage limitation (GDPR Art. 5(1)(e)):** personal data should be "kept in a form which permits identification of data subjects for no longer than is necessary" — meaning `user_profile` (and any cached IDP `/identity` response) needs an explicit retention window and deletion/expiry mechanism, not indefinite retention by default (https://complydog.com/blog/pii-data-protection-guide-personally-identifiable-information-management).

**Specific to caching third-party `/identity` responses:** since the connector's `/identity` call returns the same PII shape as `user_profile` (name, phone, address), the design question is whether to persist that response at all, or treat it as pass-through/ephemeral. Recommendation for this track's write-up:
- If cached, mark cached-from-IDP profile rows distinctly (the `source`/`origin` column from §1) so retention policy and deletion requests can be applied specifically to vendor-sourced data, independent of directly-collected profile data.
- Treat a cached IDP response as **subject to the same retention clock as directly-collected PII** — caching data doesn't reset or avoid the retention obligation; if anything, it adds a vendor-processor relationship that also needs due-diligence documentation (data flow from third-party collection through disposal) per general vendor-management guidance for third-party PII processors (https://securityscorecard.com/blog/what-is-pii-how-to-protect-personally-identifiable-information-in-2025/).
- Never let the IDP's `/auth` credential flow (username/password to the third party) touch or get stored alongside `user_credential` — that table is for *this system's own* login credentials, not the vendor's; conflating the two would mean this system persists a third party's auth secrets, which is out of scope and a clear governance violation.

**Credential/PII non-conflation, restated as a governance principle, not just a schema rule:** NIST 800-63B's guidance on salted/hashed password storage (§1) only makes sense if `user_credential` is genuinely isolated — mixing PII into that table would mean a credential-store breach also leaks profile PII, and vice versa. Keeping them structurally separate is itself a governance control, not merely a modeling convenience.

Sources:
- https://www.strac.io/blog/data-minimization
- https://www.strac.io/blog/nist-privacy-framework-data-minimization
- https://complydog.com/blog/pii-data-protection-guide-personally-identifiable-information-management
- https://securityscorecard.com/blog/what-is-pii-how-to-protect-personally-identifiable-information-in-2025/
- https://pages.nist.gov/800-63-4/sp800-63b.html

---

## 6. Recommendations for this assignment

**Schema sketch (columns, not DDL):**

`user_profile`
- `id` — UUID, primary key
- `name`
- `phone`
- `street_address`
- `locality`
- `region`
- `postal_code`
- `country`
- `source` — enum/string: `'direct'` | `'idp_cache'` (or similar), supporting retention policy targeting
- `created_at`, `updated_at`

`user_credential`
- `id` — UUID, primary key
- `user_id` — UUID, FK → `user_profile.id`
- `username` — unique
- `method_id` — FK → `auth_method.id` (lookup-table pattern, preferred over a native DB enum for portability and zero-migration extensibility)
- `secret` — salted hash (NIST 800-63B–compliant scheme, e.g. Argon2/bcrypt), nullable for methods that don't use a server-held secret (e.g. passkeys)
- `hash_algo`, `hash_cost` — metadata to support future re-hashing without a schema change
- `created_at`, `updated_at`

`auth_method` (lookup table)
- `id`, `name` (`'password'`, `'passkey'`, `'otp'`, `'oidc'`, …), `requires_secret` (bool), `is_active` (bool)

**Multi-database abstraction approach for the engineering track to implement:**
- One `ProfileRepository` and one `CredentialRepository` Go interface at the service layer; concrete implementations per backend package (a shared Postgres/CockroachDB implementation given their wire/SQL compatibility, plus a separate SQLite implementation), selected via config at startup — no ORM required, `database/sql` plus per-driver placeholder handling is sufficient.
- UUID primary keys everywhere (required for CockroachDB hotspot avoidance, portable to the other two) rather than branching PK strategy per backend.
- `INSERT ... ON CONFLICT (...) DO UPDATE` as the one upsert syntax supported by all three backends — avoid CockroachDB's `UPSERT` shorthand in shared SQL to keep queries portable.
- JSON, if used at all (e.g., a raw cached IDP payload), treated as opaque text at the DAO boundary — SQLite's and Postgres's `JSONB` formats are not interchangeable, so no JSON-operator query logic should be pushed into the DAO layer.
- Migrations: mostly one shared SQL migration series (golang-migrate or goose) using cross-compatible types, with a small number of explicitly-named per-backend override files only where required (e.g., a Postgres/CockroachDB-only trigram index on `user_profile.name`, absent in SQLite).
- Indexes: B-Tree on `phone`; trigram GIN on `name` (Postgres/CockroachDB; SQLite falls back to `LIKE` scan or FTS5); unique index on `username`; FK indexes on `user_id` and `method_id`.
- Governance: `user_credential` and `user_profile` remain structurally separate tables with no shared PII/secret columns; IDP-cached profile rows are tagged by `source` and subject to the same retention clock as directly-collected data; the vendor's own `/auth` credentials are never persisted in this schema.
