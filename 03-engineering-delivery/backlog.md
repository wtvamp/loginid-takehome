# Backlog — Track 03: Engineering & Delivery

Lead: Renata Cole. Epic: "Engineering & Delivery — Go implementation of DAO, REST API, and IDP connector" (Jira key written back here by team-lead on transcription).

Split note on the DAO: the two-package abstraction (`postgres` serving Postgres+CockroachDB, `sqlite` separate — `05-data-ops/decisions/multi-db-abstraction.md`) plus the interface/composite layer plus the cross-backend conformance suite are four genuinely separate PR-sized units, not one story with three criteria — each backend package merges independently once the interface (S2) exists, and the conformance suite (S5) only makes sense once at least two backends exist to run it against. Splitting them keeps each story reviewable on its own and matches how the work will actually land as commits.

---

## S1. Scaffolding: Go module, entrypoints, shared bootstrap, config loading

**Description:** Stand up the Go module and the two-binary skeleton the org already committed to in `PLANNING.md`'s Service boundaries section: `cmd/api-service`, `cmd/idp-connector`, both thin wrappers over a shared `internal/app` bootstrap package (route registration, config wiring) per `decisions/go-layout-debate.md`. `internal/config` loads the env-var surface named in `PLANNING.md` (`DB_DRIVER`, `DB_DSN`/`DB_DSN_FILE`, `HTTP_ADDR`, `AUTH_JWT_ISSUER`/`AUTH_JWT_AUDIENCE`, `IDP_ABC_*`). Stub handlers only — no DAO, no auth, no connector logic yet; this story's job is a buildable skeleton other stories attach to.

**Acceptance criteria:**
- [ ] The receiving track (04, reading this for containerization) can act on the resulting layout without re-deriving the reasoning in `PLANNING.md` or `decisions/go-layout-debate.md`.
- [ ] Both `cmd/*/main.go` are thin wrappers over `internal/app`; no route-registration or config-loading logic duplicated between them — enforcement site: code review against `decisions/go-layout-debate.md`'s ruling.
- [ ] `internal/config` loads every variable named in `PLANNING.md`'s Service boundaries section, with `DB_DSN_FILE` taking precedence over `DB_DSN` when both are set — enforcement site: a config-loading unit test asserting precedence.
- [ ] Module builds and both binaries start (against stub handlers) with no real backend configured.

**Depends on:** none.
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.

---

## S2. DAO interface, domain types, sentinels, factory

**Description:** Implement the interface layer from `05-data-ops/multi-db-strategy.md`'s Go-shaped contract (§1–§4): the `Repository` composite (`Profiles()`, `Credentials()`, `Methods()`, `Close()`), the three sub-interfaces, the domain structs (`UserProfile`, `UserCredential`, `AuthMethod`, `ProfileQuery`), the seven sentinel errors, and the `dao.New(driver, dsn)` factory signature. No backend logic yet — this is the shape both backend packages implement against, plus the fakes S9's tests use per `decisions/test-double-strategy.md`.

**Acceptance criteria:**
- [ ] A hire or reviewer unfamiliar with 05's reasoning can implement a backend against this package without re-reading `multi-db-strategy.md` — the Go types here are the contract, not a paraphrase of it.
- [ ] Every sentinel in 05's §4 is defined verbatim (name and wrapped message), including `ErrAlreadyExists`'s status as an internal-error signal with no caller-reachable path — enforcement site: `dao` package source, doc comment on `ErrAlreadyExists` stating that status explicitly (per Oren's finding #1, ruled by 05).
- [ ] `CreateProfileWithCredential(ctx, p, c)` exists on the composite per 05's §3b resolution, with `c.UserID` ignored on input — enforcement site: method signature plus a unit test asserting the input `UserID` is overwritten.
- [ ] Pointer fields follow "absent, never empty" — a non-nil, empty `*string` is rejected as `ErrInvalidArgument` at the boundary, not silently accepted — enforcement site: input-validation unit test per field.

**Depends on:** S1; **05: DAO Go-shaped contract** (`multi-db-strategy.md`, stable, accepted).
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.

---

## S3. Postgres/CockroachDB DAO implementation

**Description:** The `postgres` package implementing all three `Repository` sub-interfaces for both PostgreSQL and CockroachDB, per `multi-db-strategy.md`'s engine-aware retry-and-error seam requirement (§4). Includes the `crdb.ExecuteTx`-style retry wrapper on every write (CockroachDB stays at SERIALIZABLE per 05's ruling — READ COMMITTED was considered and rejected), SQLSTATE-plus-our-own-constraint-name error translation, and the transactional `CreateProfileWithCredential`.

**Acceptance criteria:**
- [ ] Implements against S2's interfaces with no re-derivation of 05's schema or error-translation reasoning.
- [ ] Every write (`Create`, `Update`, `Upsert`, `Delete`, `CreateProfileWithCredential`) runs through the retry wrapper — enforcement site: the wrapper is the single call path every write method uses, verified by the conformance suite (S5) exercising a retryable condition against CockroachDB.
- [ ] Driver errors translate to sentinels by structured code (SQLSTATE) plus our own named DDL constraints, never free-text matching — enforcement site: the translation table in `multi-db-strategy.md` §4, one `switch`/lookup per condition, no `strings.Contains` on an error message anywhere in this package.
- [ ] `Upsert`'s `ON CONFLICT DO UPDATE SET` list explicitly excludes `created_at` — enforcement site: the SQL statement itself plus a unit test asserting `created_at` survives a re-hydration Upsert unchanged (per Oren's finding #3, ruled by 05).
- [ ] Fuzzy name search uses the `LOWER(name) LIKE ?` form matching the GIN expression index, with explicit `%`/`_`/`\` escaping — enforcement site: query construction plus a unit test with a `%`-containing search term.

**Depends on:** S2.
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.

---

## S4. SQLite DAO implementation

**Description:** The `sqlite` package implementing all three `Repository` sub-interfaces, per `multi-db-strategy.md`'s dialect-difference table: app-side UUID generation before insert, `LIKE`-based fuzzy search with explicit case folding to match Postgres row sets for ASCII input, extended-result-code error translation (`SQLITE_CONSTRAINT_*`) disambiguated by the same DDL constraint names as the Postgres package.

**Acceptance criteria:**
- [ ] Implements against S2's interfaces with no re-derivation of 05's schema or dialect reasoning.
- [ ] `Create` generates the UUID in Go (SQLite has no server-side generation) before insert — enforcement site: the `Create` implementation, verified by the conformance suite asserting a populated `ID` comes back identically to the Postgres path.
- [ ] Search folds case explicitly (`LOWER(col) LIKE LOWER(pattern)`) rather than relying on SQLite's default ASCII case-insensitivity, so behavior is asserted rather than incidental — enforcement site: the query construction plus the conformance suite's cross-backend result-set comparison.
- [ ] `SQLITE_CONSTRAINT_UNIQUE`/`FOREIGNKEY`/`CHECK`/`PRIMARYKEY` map to the same sentinels as the Postgres package's SQLSTATE codes, disambiguated by the same constraint names — enforcement site: the translation table, verified by the conformance suite's sentinel-parity assertion.

**Depends on:** S2.
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.

---

## S5. Conformance suite (05's requirement, cross-backend)

**Description:** One suite, written once against the `Repository` interfaces, run against every backend implementation (postgres, cockroachdb, sqlite) — not per-package unit tests, per `multi-db-strategy.md` §7 and `decisions/multi-db-abstraction.md`. This is the enforcement site for every guarantee in 05's contract that can't be a DDL constraint: identical `Search` result sets/ordering across backends (closing the documented SQLite `LIKE` case-sensitivity correctness gap, not a performance one), sentinel parity via `errors.Is` for every condition in §4, the secret-state rules (§5/§6.11), `is_active` enforcement (§6.10), and empty-string rejection on every `*string` filter.

**Acceptance criteria:**
- [ ] The suite is written once against the interface and executed against all three backends from one test file/table, not duplicated per package.
- [ ] Asserts identical `Search` result sets and ordering across backends for ASCII input — enforcement site: this suite is the enforcement site named in `multi-db-strategy.md` for this guarantee; there is no other.
- [ ] Asserts `errors.Is` sentinel parity for every condition in the §4 translation table, across all three backends.
- [ ] Asserts the `requires_secret`/`secret_state` relationship (§6.11) and `is_active` (§6.10) — the two conditions 05 named as unable to have a DDL enforcement site.
- [ ] CI (04's pipeline) runs this suite against every backend including CockroachDB, not just Postgres+SQLite — flagged to 04 if their pipeline doesn't.

**Depends on:** S3, S4.
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.

---

## S6. REST handlers: search/retrieve user_profile (Q2)

**Description:** The client-facing handlers in `internal/api` implementing the assignment's Q2 (search and retrieve `user_profile`), built against S2's interfaces (fakes stand in for a real backend per `decisions/test-double-strategy.md`, so this doesn't wait on S3/S4). Implements the object-level `authorize(sub, scope, target)` call as an explicit in-handler step, not a second middleware, per the ruling in `handoff-03-auth.md` v2 (my own answer, confirmed by 02).

**Acceptance criteria:**
- [ ] Implements `handoff-03-auth.md` v2's scope vocabulary and pagination/rate-limit numbers without re-deriving 02's authz-scoping reasoning.
- [ ] `authorize()` runs as an explicit call inside each handler after request parsing, before the DAO call — enforcement site: handler code structure, per the go-layout-debate.md-adjacent ruling recorded in this track's reply to 02.
- [ ] Cursor-based pagination only, default page size 20, hard ceiling 50 — enforcement site: the pagination parameter parsing, rejecting any offset-based parameter.
- [ ] Unmapped DAO errors never surface `err.Error()` to the client; only the mapped sentinels' safe messages do — enforcement site: the boundary translation function from `decisions/error-semantics.md`.
- [ ] `profile:read:any` requests carry a `reason_code` from the closed enum on every call, logged with the audit record.

**Depends on:** S2; **02: Auth scheme and authorization hand-off** (`handoff-03-auth.md` v2, accepted).
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `security-graded`, `no-code-yet`.

---

## S7. Auth middleware (JWT validation)

**Description:** The middleware at the `cmd/api-service` edge validating the client-credentials JWT scheme from `handoff-03-auth.md` v2 — public-key-only verification, `sub`/`scope`/`exp`/`iat`/`aud` claim checks, `kid`-based key rotation, no denylist (TTL-only revocation, a deliberate trade-off not to be re-litigated).

**Acceptance criteria:**
- [ ] Implements 02's token scheme without re-deriving the no-refresh/no-denylist reasoning in `api-auth-design.md`.
- [ ] Rejects any token whose `aud` doesn't match this service's audience — enforcement site: the claim-validation step, with a unit test using a mismatched-audience token.
- [ ] Verifies with the public key only; the private signing key never touches this binary's code path — enforcement site: dependency/import check (no signing-key material referenced in `internal/api`).
- [ ] Per-client-credential rate limits (60 req/min read, 10 req/min search) and the cumulative distinct-record-touch counter (rolling 24h) are enforced and alert on sustained near-limit usage, not just throttle — enforcement site: the rate-limit middleware plus its own test.
- [ ] The deadline budget for a request (context propagation into the DAO layer) is sized to cover realistic p99 DAO latency, per `decisions/context-propagation.md`'s ruling — stated as a concrete number in this story's implementation, not left as "some deadline."

**Depends on:** S1; **02: Auth scheme and authorization hand-off** (`handoff-03-auth.md` v2, accepted).
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `security-graded`, `no-code-yet`.

---

## S8. Connector: /auth + /identity (Q3)

**Description:** `cmd/idp-connector`'s outbound client and inbound handlers for the assignment's fixed `/auth` and `/identity` contracts, per `connector-security.md`'s no-cache-by-default token lifecycle (fetch per `/identity` call, discard on return; fresh isolated `Authorization` header per outbound request, never inherited from pooling/keep-alive; fetch → use-once → zeroize, retries re-fetch via `/auth`). Includes the connector's own inbound self-auth (distinct audience `idp-connector-service`, scope `connector:identity-lookup`, restricted to `api-service` as caller) per `handoff-03-auth.md` v2 §"Where it's validated" (Ingrid's S8 catch), and the `context.WithTimeout` + bounded pool + backoff requirement from `decisions/go-layout-debate.md`.

**Acceptance criteria:**
- [ ] Implements 02's token-lifecycle spec (`connector-security.md`, `decisions/connector-token-lifecycle-redblue.md`) without re-litigating the no-cache-by-default posture.
- [ ] No vendor token is persisted or cached across requests by default; if a future exception is ever justified it requires re-review, not a silent default change — enforcement site: no storage/cache call in the token-fetch path.
- [ ] Every outbound vendor call sets `context.WithTimeout`; the outbound HTTP client uses a bounded connection pool; token-refresh retries back off rather than tight-looping — enforcement site: the HTTP client construction plus a test asserting a timeout fires.
- [ ] The connector's own `/auth`/`/identity` handlers validate an inbound JWT with `aud: idp-connector-service` and `scope: connector:identity-lookup`, distinct from `api-service`'s audience — enforcement site: this handler's own middleware instance, with a test asserting a `loginid-api-service`-audience token is rejected here.
- [ ] Never logs, in any environment or error path: the end-user's vendor password, any vendor `access_token` or full `Authorization` header value, or raw vendor error response bodies (per `handoff-04-secrets.md`'s never-log list) — enforcement site: log-call review against that list.

**Depends on:** S1; **02: Connector security spec** (`connector-security.md`, accepted); **02: Auth scheme and authorization hand-off** (`handoff-03-auth.md` v2, accepted, for the connector's own self-auth).
**Model/effort:** opus, high — the only implementation seat at this tier, per `planning-approach.md`'s connector-token-lifecycle exception.
**Type:** Story.
**Labels:** `track-03`, `security-graded`, `no-code-yet`.

---

## S9. Handler and connector unit tests

**Description:** Table-driven unit tests for the handler layer (S6/S7, mocked DAO via S2's hand-written fakes) and the connector (S8, mocked HTTP), per `decisions/test-double-strategy.md`. The DAO's own cross-backend correctness is S5's job, not this story's — this covers request/response shape, auth rejection paths, and error-translation-to-HTTP-status mapping.

**Acceptance criteria:**
- [ ] Test doubles are hand-written interface fakes, not a generated-mock library — per the ruling in `decisions/test-double-strategy.md`.
- [ ] Any change to `Repository` or its sub-interfaces updates the corresponding fake in the same PR — enforcement site: review checklist item, stated in this story's PR template/description.
- [ ] Handler tests cover every sentinel-to-HTTP-status mapping from `decisions/error-semantics.md`'s boundary translation function, including the unmapped-error-defaults-to-500-with-no-detail case.
- [ ] Connector tests cover the no-cache token-lifecycle assertions (a second `/identity` call re-fetches via `/auth`, never reuses a prior token) with mocked HTTP.

**Depends on:** S6, S7, S8.
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.

---

## S10. README: implemented vs. stubbed, AI workflow, test strategy

**Description:** The track's README stating what's implemented vs. stubbed/mocked and why, the AI tool/workflow used to produce this track's code (Claude Code, this org's hiring and orchestration-pattern structure — `PLAN.md`, `decisions/`), and a test-strategy section citing `decisions/test-double-strategy.md`'s same-PR fake-update rule explicitly, per the standing note from team-lead.

**Acceptance criteria:**
- [ ] A cold reader can tell what runs vs. what's a stub without asking — the same bar as every hand-off in this project.
- [ ] Test-strategy section states unit-heavy, integration deliberately stubbed (the assignment doesn't require full end-to-end), and names the same-PR fake-update rule from `decisions/test-double-strategy.md` explicitly.
- [ ] AI tooling note names the model/effort tiers actually used per story (from this backlog) and the orchestration patterns run (`decisions/go-layout-debate.md`, `decisions/error-semantics.md`, `decisions/context-propagation.md`, `decisions/test-double-strategy.md`), not just that "Claude Code was used."

**Depends on:** S1–S9.
**Model/effort:** sonnet, high.
**Type:** Story.
**Labels:** `track-03`, `no-code-yet`.
