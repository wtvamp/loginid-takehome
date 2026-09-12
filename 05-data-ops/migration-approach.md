# Migration Approach

Owner: 05-data-ops (Priya Nandakumar). **Primary receiver: `../04-infra-devops/`** — this is one of the three inputs that track is gated on. Secondary receiver: `../03-engineering-delivery/`, which owns the migration files themselves alongside the DAO code.

Written to the project's hand-off standard: 04 should be able to act on this without re-deriving my reasoning. Where I state a *requirement*, the mechanism is 04's to choose; where I state a *mechanism*, it is because the requirement cannot be met without it, and I say why.

Status: **final.** The devil's-advocate objection turn (story S4 in `./PLAN.md`) ran and raised six objections; all six were accepted. The two I named as most worth attacking — the tool choice and the single-runner requirement — were indeed where it landed: R2's *conclusion* survived but its *reasoning* was wrong and has been replaced, and R3 was found to be checking one version number where §2 creates two. Dispositions: `./decisions/migration-approach-objections.md`.

## 1. Tool: goose

**goose** — a Go command-line migration runner that applies numbered `.sql` files in order and records which have been applied in a version table inside the target database. Plain-SQL-first, minimal dependencies. Reasoning is in `./multi-db-strategy.md` and `./research-data-ops-best-practices.md` §3: this schema is "mostly shared DDL, occasionally per-backend", and goose's loose directory model fits that better than golang-migrate's stricter paired up/down files. golang-migrate is an equally defensible tool and I will not contest a swap if 03 already has tooling preferences — **nothing in §3–§6 below changes if the tool changes — with one named exception**: §2's two-invocation mechanism uses goose's `-table` flag, so a swap to golang-migrate means re-deriving how two independent version sequences are kept apart. That is the tool-independence claim's only load-bearing use, and pretending otherwise would be the kind of small overclaim this document is otherwise trying to avoid. Atlas is the schema-as-state alternative worth knowing about; it is more than this take-home needs.

Version numbering: timestamped filenames during development (avoids collisions when two branches add a migration), converted to a clean sequential 001/002/003 series with `goose fix` — the subcommand that renumbers timestamped files — **before the migrations have been applied anywhere that persists.** That cutoff is the important half: renumbering after application rewrites history the version table already records, so `goose fix` is a pre-first-application step, not a tidy-up before tagging. Not a decision 04 needs to act on — noted so nobody "fixes" the inconsistency later.

## 2. Layout

```
migrations/
  shared/     # seed data only — no CREATE TABLE
  postgres/   # all DDL for Postgres + CockroachDB, plus the trigram index
  sqlite/     # all DDL for SQLite
```

**Corrected after the cross-track consistency pass (F28).** This section previously put every `CREATE TABLE` in `shared/`, with `postgres/` holding one index file. That was wrong, and wrong in a way the rest of this track's own work had already ruled out: the field-level schema in `multi-db-strategy.md` §5 gives **every** table per-engine DDL — `UUID DEFAULT gen_random_uuid()` vs `TEXT`, `TEXT COLLATE "C"` vs `TEXT COLLATE BINARY`, a regex `CHECK` vs a `GLOB` `CHECK`, `TIMESTAMPTZ` vs `TEXT`, `BYTEA` vs `BLOB`, `BOOLEAN` vs `INTEGER`. A single shared `CREATE TABLE` series cannot express that. SQLite's type affinity would quietly swallow the differing *type names*, which is what made the original claim look survivable, but it does nothing for collation clauses or CHECK syntax — those are hard syntax differences, not affinity.

The document already carried the right answer as a hypothetical fallback ("per-backend `CREATE TABLE` files and `shared/` reduced to seed data") and left it contingent on an open item marked "under review in story S1". **S1 closed, the review resolved it in favour of the fallback, and this document was not updated** — so a document marked "final" was carrying a provisional default that its own dependency had already overruled. That is the failure, and it is worse than the layout error: the fallback was correct, written down, and left unpromoted.

So:

- **`shared/`** carries the `auth_method` seed row and nothing else. No `CREATE TABLE`.
- **`postgres/`** carries the full DDL for `auth_method`, `user_profile`, `user_credential` and `deletion_log` in Postgres/CockroachDB types, plus the `pg_trgm` trigram expression index on `LOWER(name)`.
- **`sqlite/`** carries the same four tables in SQLite types and collations, and no trigram index — there is no SQLite equivalent and it is omitted rather than faked.

**Nothing in §3–§6 changes, and neither does 04's design**, exactly as the earlier text promised: the two-invocation mechanism, the separate version tables, R1–R5 and the seed-data rule are untouched. R3's worked example still holds and in fact reads better now — a service can have `shared/` current with the `pg_trgm` index unapplied, which is why fail-closed must check both version tables. (`pg_trgm` is a PostgreSQL extension that indexes three-character fragments of text so that substring searches like `LIKE '%smith%'` can use an index instead of scanning every row — it is what makes name search fast. CockroachDB has the same capability built in; SQLite has no equivalent, so the index is omitted there rather than faked.). That parity gap is documented and deliberate (`./multi-db-strategy.md`); SQLite is the local/dev/demo backend, not a production peer.

**Mechanism 04 needs:** goose applies one directory per invocation, so a migration run is **two invocations** — `shared/` then the backend directory — and each needs its own version table (`goose -table goose_db_version_shared`, then `-table goose_db_version_<driver>`). One shared table across two directories would interleave version numbers from independent sequences and corrupt the history. Both invocations must succeed for the run to count as successful.

**That open item is now closed.** It asked whether a single `shared/` DDL series survives all three backends, given that SQLite does not enforce column types the way PostgreSQL does — it assigns each column a loose "type affinity" and falls back to NUMERIC for any type name it does not recognise, `UUID` included. The answer turned out to be no, and for a reason narrower than type affinity: affinity would have absorbed the type names, but collation clauses and CHECK syntax differ outright. Resolved in favour of the per-backend split above. No open items remain in this document.

## 3. Requirements on 04 — the part I actually need decided

**R1. Migrations run as a separate step that completes before any service replica starts.** Not on service boot. Two replicas booting together would both attempt to migrate, and see R2 for why the database will not save you from that. A dedicated job/init step that must exit zero before the service rolls.

**R2. Exactly one migration runner at a time, enforced by the platform — not by the database.** This is the requirement I most want 04 to read carefully, because the obvious mechanism does not work here. An *advisory lock* is a named lock an application asks the database to hold on its behalf; the database enforces nothing about what it protects, so it is purely a convention between cooperating clients — the usual way to stop two copies of a job running at once. `pg_advisory_lock` is the standard answer on PostgreSQL, but on CockroachDB **session-scoped advisory locks error out, and the `pg_try_advisory_lock` variants silently no-op** — a try-lock returns as though it succeeded and lets a second runner proceed (sourced in `./research-cockroachdb-postgres-semantics.md` §5; only transaction-scoped `pg_advisory_xact_lock` works there). On SQLite the concept is meaningless. A silent false success is a worse failure than an error. **The reason this argument originally gave was wrong, and is replaced.** The first version reasoned "a locking strategy that only works on one backend is not a strategy." That was true of a premise since corrected: `pg_advisory_xact_lock` *does* work on CockroachDB, and SQLite is a single-file local backend where concurrent migration runners are not a scenario that exists — so a transaction-scoped lock works on both backends that can actually have replicas. An objection turn caught that I had over-rotated one research finding into a platform requirement.

**The conclusion survives on a different and better reason: a transaction-scoped lock cannot cover a migration run.** `pg_advisory_xact_lock` is released when its transaction ends, and goose applies each migration file in its own transaction. The lock would therefore be released *between files*, letting a second runner interleave mid-run — which is precisely the failure it was supposed to prevent. Session-scoped locks, which would span the whole run, are the ones CockroachDB does not support. So the database can offer either the right scope or the right engine coverage, never both.

So mutual exclusion has to come from the orchestration layer — a job with parallelism 1, a pipeline stage that cannot run concurrently with itself, whatever 04's platform offers. **This is a genuinely open question handed to 04, not a resolved item written up for information.** If 04 concludes the platform cannot guarantee single-runner execution, tell me and I will treat it as a real constraint on the design rather than a detail.

**A cheap portable backstop worth adding regardless — and an honest gap.** goose's version table is the one artifact all three backends share, so a unique constraint on its `version_id` would make a second concurrent runner fail on insert: portable, free, no platform feature required. **Whether goose's default version table already carries that constraint is unverified here** — I am not asserting it either way, because the objection that raised this correctly pointed out that the document claimed the database could not help without examining the one database-level mechanism goose already creates. The actionable form does not depend on the answer: **add a unique index on `version_id` ourselves in the first migration**, which is portable across all three backends and costs nothing if goose already does it. This is a backstop, not a replacement for R2 — it turns a silent interleave into a loud failure, which is the property actually worth buying.

**R3. The service fails closed when the schema is behind — checking BOTH version tables.** §2 creates two independent version sequences (`shared/` and the backend directory), so "the applied version" is two numbers, not one. The check must compare both, and must know which backend directory applies to the engine it is connected to. Checking only `shared/` would let a service pass with the `pg_trgm` index unapplied — caught by an objection turn, and it would have been a passing check that proved nothing. On startup the service compares both applied versions against the versions it was built against and refuses to serve if either is behind, rather than serving traffic against a schema missing a column it will query. The requirement is mine; whether the expected version is embedded at build time or derived from the migration files shipped in the image is 03's and 04's call.

**R4. Two distinct database credentials, not one.**
- *Migration runner*: DDL rights (`CREATE`/`ALTER`/`DROP`, index creation) plus, on PostgreSQL only, whatever `CREATE EXTENSION pg_trgm` requires — on managed Postgres this is usually an allow-listed extension, otherwise it needs elevated privileges or the platform pre-installing it. CockroachDB needs nothing here; trigram indexes are native. SQLite has no privilege model at all.
- *Runtime service user*: DML only (`SELECT`/`INSERT`/`UPDATE`/`DELETE`). **No DDL.** A running service has no legitimate reason to alter the schema, and a service account that can `DROP TABLE user_profile` turns a SQL-injection finding into data destruction rather than data disclosure.

Both credentials belong on 02's secrets inventory; I am flagging the requirement, not claiming that surface.

**R5. Rollback is not the recovery plan for data.** Down migrations will exist for the base schema and are genuinely useful in development. They are not a recovery mechanism in production: reversing a migration that dropped a column does not bring the data back, and the data in question here is PII. Recovery from a destructive migration is backup-restore, and backup policy is 04's. I raise it because "we have down migrations" reads like a safety net and is not one.

## 4. Seed data

The `auth_method` table is seeded with one row, `"password"`. It ships as a migration file in `shared/`, not as a runtime bootstrap in the service — a service that writes its own lookup rows on boot is a service that can write them wrongly, concurrently, on every replica.

Idempotent via `INSERT ... ON CONFLICT (name) DO NOTHING`, which the unique constraint on `auth_method.name` already supports and which all three backends accept (the same portable upsert form the DAO uses — see `./multi-db-strategy.md`). goose will not re-run an applied migration anyway; the `ON CONFLICT` makes the file safe against a database someone seeded by hand, which in practice is how this breaks.

New auth methods after `"password"` — `passkey`, `otp`, `oidc` — are **inserted as data, not added as migrations**. That is the entire point of the lookup-table design over a native enum: adding a supported method is an operational action, not a schema change and not a deploy. If 04 finds itself asked to write a migration to add an auth method, that is a signal something has been misunderstood upstream — flag it to me.

## 5. Explicitly not owned by this document

Named so 04 does not read a boundary into this doc that isn't there: where the databases actually run; how the pipeline invokes goose; environment topology and how many of them there are; backup schedule and retention of backups; connection pooling; monitoring of migration runs. All 04's. This document says *what must be true* about migration execution and *why*, and stops there.

Separately not mine: the **retention sweep and deletion job** are governance mechanisms, not schema migrations, and they are specified in `./pii-governance.md` (policy shape) — not here. Do not implement retention as a migration.

## 6. The abstraction ruling — now closed, and it changes nothing here

Story S3 in `./PLAN.md` — the ruling on the multi-database abstraction shape — **is now ruled**: per-driver implementations behind one interface, confirmed. Full reasoning in `./decisions/multi-db-abstraction.md`. Codegen (sqlc) turned out to be orthogonal to the question rather than a competing option, and is left to 03 as an implementation preference.

This section originally said "nothing in this document is waiting on S3," and that held: sqlc generates Go from SQL and does not manage schema versions, so the layout, the two-invocation mechanism, R1–R5 and the seed-data rule were never exposed to the outcome either way. **Nothing in this document changed when S3 closed.** It is recorded rather than deleted so 04 can see that the dependency was correctly assessed rather than quietly dropped.

One thing did change, in R2, and it came from research rather than from S3 — see the advisory-lock correction there.

---

*AI tooling note: drafted by Claude Opus 5 in the 05-data-ops track-lead session, from this track's own research doc and strategy doc, then put through a single devil's-advocate objection turn (Beatriz Achterberg, Spreadsheet archetype, sonnet) before hand-off. Per `../CASTING.md` §5, the objection and its disposition are recorded in `./decisions/`.*
