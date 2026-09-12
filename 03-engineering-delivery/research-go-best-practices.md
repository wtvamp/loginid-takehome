# Research: Current Go Best Practices for This Assignment (2025-2026)

This document informs the eventual implementation of the three coding questions owned by this track (see `./CLAUDE.md` and `../CLAUDE.md`). It does not contain implementation code. Research was done via web search in September 2026; sources are cited inline.

## 1. Project Layout

There is no official Go project layout. The Go team has repeatedly said the popular [`golang-standards/project-layout`](https://github.com/golang-standards/project-layout) repo (50k+ stars) is a community convention, not an endorsed standard, and it is widely criticized in 2025-2026 writeups for being over-structured for small/medium services.

Current consensus, per [reintech.io](https://reintech.io/blog/go-project-structure-2026-clean-architecture-best-practices), [oneuptime.com](https://oneuptime.com/blog/post/2026-01-07-go-project-structure/view), and [learn.appliedgo.net](https://learn.appliedgo.net/blog/go-project-layout):

- Start flat. A single package at the module root is idiomatic and sufficient for small programs.
- Add structure only when the code demands it, not up front from a template. Every added directory should answer a specific pressure (multiple binaries, code you want to hide from external importers, etc.).
- `cmd/<binary>/main.go` is the one convention worth adopting early when there is more than one entry point (e.g., the API server and the IDP connector service in this assignment are plausibly two separate binaries).
- `internal/` is the other convention worth adopting early — it is compiler-enforced (Go tooling refuses external imports of `internal/` packages), so it is not just a style preference, it's a real boundary. Good fit for this assignment's DAO and auth internals, which should not be importable outside this module.
- Avoid pre-declaring `pkg/`, `api/`, `configs/`, etc. per the golang-standards template unless a concrete need appears — this reads as cargo-culting in a small interview-scale codebase and a reviewer familiar with Go idioms will notice.

**Relevance to this assignment:** given three semi-independent deliverables (DAO, REST API, IDP connector), a defensible layout is:

```
03-engineering-delivery/
  cmd/
    api/            # question 2 entrypoint
    idpconnector/    # question 3 entrypoint
  internal/
    dao/             # question 1: repository interfaces + postgres/sqlite implementations
    auth/            # jwt/middleware, shared by cmd/api
    idp/             # question 3: provider client(s), /auth and /identity handlers
  go.mod
```

This is minimal, each directory answers a real need (two binaries, hidden internals), and doesn't over-claim structure the assignment doesn't need.

## 2. DAO / Repository Pattern for Multi-Database Support

Sources: [reintech.io sqlc vs GORM vs sqlx](https://reintech.io/blog/sqlc-vs-gorm-vs-sqlx-go-database-libraries-compared-2026), [Encore: comparing Go ORMs](https://encore.cloud/resources/go-orms), [JetBrains Go blog](https://blog.jetbrains.com/go/2023/04/27/comparing-db-packages/), [dasroot.net](https://dasroot.net/posts/2025/12/go-database-patterns-gorm-sqlx-pgx-compared/), sqlc docs on [managed databases](https://docs.sqlc.dev/en/latest/howto/managed-databases.html) and [changelog](https://docs.sqlc.dev/en/latest/reference/changelog.html), CockroachDB docs on [driver compatibility](https://www.cockroachlabs.com/docs/stable/install-client-drivers), [pgx repo](https://github.com/jackc/pgx).

Options and trade-offs:

- **`database/sql` directly** — zero dependencies, maximum control, but verbose (manual `Scan` into every field, manual `Rows.Close()` handling), and every query is stringly-typed with no compile-time safety. Fully idiomatic but a lot of boilerplate to hand-write and hand-review in a take-home.
- **`sqlx`** — thin extension of `database/sql` (`StructScan`, named parameters). Keeps the mental model of "you write the SQL" while removing most boilerplate. Does not generate code or enforce a schema; it is a convenience layer, not an abstraction layer. Widely regarded as "idiomatic Go with less pain."
- **`sqlc`** — generates type-safe Go from hand-written SQL + schema. Excellent type safety and no runtime reflection/magic, but as of the 2026 docs, sqlc's engine support differs by database and its newer schema/database-aware analysis features are Postgres-first (MySQL/SQLite support for some analyzer features is still catching up per the changelog). It also generates separate code per target engine rather than one implementation behind an interface automatically — you still write the interface abstraction yourself, sqlc just generates the per-engine implementation bodies. That's a reasonable fit if willing to maintain near-duplicate SQL files per engine (Postgres dialect vs SQLite dialect), but it adds a build-time code-gen step that's more to explain in a take-home.
- **GORM** — full ORM, struct-tag driven, supports Postgres/SQLite/MySQL/SQL Server dialects out of the box via dialect drivers, and is the easiest to make "just work" across two databases with one model definition. Trade-off: it hides SQL behind an abstraction with real behavioral differences per dialect (e.g., upsert semantics, migrations, N+1-prone lazy patterns) — the "magic" the assignment explicitly says to avoid favoring.
- **ent** — Facebook-style code-first ORM/graph library. Powerful for complex relational graphs, but it's the heaviest and least idiomatic-reading option, and this assignment's schema (two flat tables) doesn't need graph traversal or ent's codegen machinery. Overkill for the actual data shape here.

**CockroachDB note:** CockroachDB speaks the Postgres wire protocol, so the standard `pgx` driver (or `lib/pq` / `database/sql` with a Postgres driver) works against both Postgres and CockroachDB unchanged — the two are not really "two databases" from the driver's point of view, only SQLite is a genuinely different dialect (no `SERIAL`, different placeholder/type behavior, file-based).

**Recommended pattern regardless of library choice:** define a `Repository` interface (e.g., `UserProfileRepository`, `UserCredentialRepository`) in `internal/dao`, with one Postgres/CockroachDB implementation (via `pgx` or `database/sql` + `sqlx`) and one SQLite implementation (via `mattn/go-sqlite3` or the pure-Go `modernc.org/sqlite`), both satisfying the same interface, selected at startup via config/driver name. This is the "hexagonal"/ports-and-adapters shape reviewers expect and is trivial to explain and partially stub.

## 3. REST API Framework Choice

Sources: [reintech.io Chi vs Gin vs Echo 2026](https://reintech.io/blog/go-chi-vs-gin-vs-echo-web-framework-comparison-2026), [Encore: best Go backend frameworks](https://encore.dev/articles/best-go-backend-frameworks), [Encore: Chi alternatives](https://encore.dev/articles/chi-alternatives), [developersvoice.com](https://developersvoice.com/blog/go/building_high_performance_go_apis/).

- **Go 1.22+ `net/http`** added pattern-based routing with method matching and path parameters (`mux.HandleFunc("GET /users/{id}", ...)`), closing most of the gap that used to justify a router dependency. For a small, interview-scale service with a handful of routes, this is now genuinely viable with zero third-party dependencies.
- **chi** — a thin, idiomatic router built directly on `net/http` (handlers are still `http.Handler`), adds middleware chaining, route groups, and sub-routing without diverging from the standard library's types. Widely recommended in 2026 write-ups as the "if you want something, want this" middle ground.
- **gin** — largest ecosystem/community, own context type (`*gin.Context`) rather than `http.Handler`, built-in JSON binding/validation. Good for teams optimizing for ecosystem/velocity, less "plain Go" in style.
- **echo** — similar feature set to gin, considered slightly more idiomatic-reading and with good OpenAPI tooling support.

**Consensus for a small service:** performance differences are negligible once real work (DB calls, auth) dominates; the choice is about API surface and dependency footprint, not throughput.

## 4. Password / Credential Handling

Sources: [reintech.io Argon2 vs bcrypt vs scrypt 2026](https://reintech.io/blog/password-hashing-2026-argon2-bcrypt-scrypt-comparison), [Alex Edwards: Argon2 in Go](https://www.alexedwards.net/blog/how-to-hash-and-verify-passwords-with-argon2-in-go), [go-tools.org](https://go-tools.org/blog/bcrypt-vs-argon2-vs-scrypt-password-hashing).

- 2026 guidance: default to **Argon2id** (`golang.org/x/crypto/argon2`) with OWASP baseline parameters (roughly `m=19456 (19 MiB), t=2, p=1`), embedding the parameters + salt in the stored hash string so they can evolve without a migration. No 72-byte input cap (unlike bcrypt).
- **bcrypt** (`golang.org/x/crypto/bcrypt`) remains acceptable and is still the most commonly seen choice in Go tutorials/production code because of its one-call, no-parameter-tuning API (`bcrypt.GenerateFromPassword` / `CompareHashAndPassword`) and long track record; its main limitations are the 72-byte input truncation and fixed, less GPU-resistant cost model versus Argon2id.
- Both live under `golang.org/x/crypto`, both are "standard enough" that a reviewer will recognize either as a defensible choice; Argon2id is the more current, more defensible pick to name explicitly in 2026, with bcrypt as an acceptable, simpler fallback if minimizing new dependencies/tuning surface for a take-home.
- **Schema fit:** the assignment's `user_credential` table already has a `method` field — this is exactly the extension point recommended in the sources for supporting more than one credential method later (e.g., `method='password'` now, `method='webauthn'`/`method='passkey'` in the future, which is directly relevant given this is a LoginID/passwordless-auth-adjacent interview). Never store the raw password; store only the hash (and the algorithm/parameters) in the field currently named `password`, and document that naming choice explicitly since it's slightly misleading — a reviewer will look for this.

## 5. API Authentication Middleware Patterns

Sources: [WorkOS: Go authentication guide 2026](https://workos.com/blog/go-authentication-guide), [golang-jwt/jwt](https://github.com/golang-jwt/jwt) (community-maintained successor to `dgrijalva/jwt-go`, which is unmaintained), [auth0/go-jwt-middleware](https://github.com/auth0/go-jwt-middleware).

- **`golang-jwt/jwt/v5`** is the de facto standard JWT library in the Go ecosystem in 2026 — full RFC 7519 support, actively maintained, direct successor to the abandoned `dgrijalva/jwt-go`. Any legacy code/tutorials still importing `dgrijalva/jwt-go` should be treated as outdated.
- Standard middleware shape: a handler wrapper that reads the `Authorization: Bearer <token>` header, verifies signature + expiry via `jwt.ParseWithClaims`, and on success stores validated claims in the request `context.Context` (via a private context-key type) for downstream handlers to read; on failure, returns `401`. This pattern is identical whether built on plain `net/http`, chi, gin, or echo — only how the middleware is registered differs.
- Production guidance favors short-lived access tokens (15 min-1 hr) with a longer-lived refresh token, though for a take-home scope a single access token with a stated expiry is a reasonable, explicitly-scoped simplification — say so rather than silently building a partial refresh flow.
- This library choice and pattern should be treated as the concrete mechanism implementing whatever auth *design* `../02-ai-security-architecture/CLAUDE.md` specifies (this track does not own the auth design, only its Go implementation).

## 6. Testing Conventions

Sources: [oneuptime.com table-driven tests 2026](https://oneuptime.com/blog/post/2026-01-07-go-table-driven-tests/view), [dasroot.net Go testing excellence 2026](https://dasroot.net/posts/2026/01/go-testing-excellence-table-driven-tests-mocking/), [BackendBytes Go testing best practices](https://backendbytes.com/articles/go-testing-best-practices/), [Rost Glukhov: Go unit testing structure](https://www.glukhov.org/app-architecture/testing-architecture/unit-tests-in-go).

- **Table-driven tests** (`[]struct{name string; input ...; want ...}` iterated with `t.Run(tc.name, ...)`) are the expected default shape for any function with multiple input/output cases — this is what a Go-familiar reviewer looks for first.
- **`httptest.NewRecorder`** for unit-testing individual HTTP handlers (call the handler function directly against a fake `ResponseWriter`/`*http.Request`), and **`httptest.NewServer`** when a full round trip through middleware/routing is needed (e.g., verifying the JWT middleware actually rejects a missing/expired token end-to-end).
- **DB mocking:** because the DAO layer is defined as an interface (§2), tests for the API/service layer can use a hand-written or generated (e.g., via `testify/mock` or `mockery`) fake implementing the same repository interface — no real database needed for handler-level tests. Reserve real-database integration tests (optionally via `testcontainers-go` spinning up ephemeral Postgres/SQLite) for a smaller set of DAO-layer tests that verify the actual SQL against a real engine; this split (unit tests against mocked interfaces, a few integration tests against real engines) is the expected 2026 convention and maps cleanly onto "tests don't need full coverage, but the approach should be stated" from this track's brief.
- Given the assignment explicitly allows partial/non-complete code, the bar a reviewer will use is: does at least one table-driven test exist per package, does at least one handler test use `httptest`, and is the DAO interface demonstrably mockable — not exhaustive coverage.

## Recommendations for This Assignment

A concrete, opinionated stack, chosen to be defensible and explainable in an interview setting — favoring idiomatic clarity over magic, per the assignment's explicit steer:

| Concern | Choice | Why |
|---|---|---|
| Project layout | Minimal: `cmd/api`, `cmd/idpconnector`, `internal/dao`, `internal/auth`, `internal/idp` | Matches actual need (two binaries, hidden internals) without cargo-culting the golang-standards template; easy to justify line-by-line in an interview. |
| DB access (question 1) | `database/sql` + `sqlx` for both backends, one `Repository` interface, two implementations (Postgres/CockroachDB via `pgx` stdlib driver, SQLite via `modernc.org/sqlite`, pure-Go, no CGO) | No code-gen step to explain, no ORM "magic" hiding dialect differences, and the interface + two-implementation shape is the clearest possible demonstration of "multi-database support behind one interface" for a reviewer. CockroachDB needs no separate driver — Postgres wire protocol covers it. sqlc was considered but rejected here specifically because its schema/engine-aware tooling is Postgres-first, adding asymmetry between the two backends that's awkward to defend in a take-home. GORM was considered and rejected because its dialect abstraction is exactly the kind of "magic" the assignment says to avoid favoring. |
| REST framework (question 2) | Go 1.22+ `net/http` alone, no router dependency | The route surface here is tiny (search/retrieve endpoints); Go 1.22's method+path-pattern routing covers it with zero dependencies, which is the strongest possible signal of "idiomatic Go" for a reviewer, and sidesteps any framework-preference debate entirely. If the route count grows meaningfully, `chi` is the fallback (stays on `http.Handler`, adds only middleware/sub-routing). |
| Password hashing | `golang.org/x/crypto/bcrypt`, `method="password"` in `user_credential` | Simpler, zero-tuning API than Argon2id, still fully defensible as current-enough practice, and the schema's `method` column is the explicit extension point for future non-password methods — worth calling out given this is a LoginID/passwordless interview. Argon2id noted in the doc above as the more cutting-edge alternative if asked "why not Argon2." |
| API auth (question 2 middleware) | `golang-jwt/jwt/v5`, bearer-token middleware storing claims in request context | Community-standard, actively maintained JWT library; the middleware pattern is framework-agnostic so it works unchanged whether `net/http` or `chi` is used. Implements (does not redesign) whatever `../02-ai-security-architecture/CLAUDE.md` specifies. |
| Third-party IDP connector (question 3) | Plain `net/http` handlers for `/auth` and `/identity`, `golang-jwt` only if the connector itself issues tokens (vs. just proxying vendor tokens) | Keeps question 3 consistent in style with question 2; no new framework needed for two endpoints. |
| Testing | Table-driven unit tests per package; `httptest.NewRecorder` for handler tests; hand-written fakes against the `Repository` interface for service-layer tests; a small number of real-engine tests (Postgres via Docker, SQLite in-memory) if time allows | Matches 2026 community convention, gives a reviewer the expected signals (interface-based mocking, table-driven cases, `httptest` usage) without requiring full coverage, and is honest about what's stubbed vs. real per the track's stated deliverable. |

This stack requires no code-generation tooling, no ORM behavior to explain away, and every dependency (`sqlx`, `pgx`, `modernc.org/sqlite`, `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`) is a widely-recognized, actively-maintained, single-purpose library — the implementation can proceed directly from this document without revisiting these choices.
