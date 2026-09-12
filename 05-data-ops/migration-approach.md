# Migration Approach

Owner: 05-data-ops (Priya Nandakumar). **Primary receiver: `../04-infra-devops/`** — this is one of the three inputs that track is gated on. Secondary receiver: `../03-engineering-delivery/`, which owns the migration files themselves alongside the DAO code.

Written to the project's hand-off standard: 04 should be able to act on this without re-deriving my reasoning. Where I state a *requirement*, the mechanism is 04's to choose; where I state a *mechanism*, it is because the requirement cannot be met without it, and I say why.

Status: draft, pending one devil's-advocate objection turn (S4 in `./PLAN.md`). The tool choice and the single-runner requirement are the two things most worth attacking.

## 1. Tool: goose

`goose` (plain-SQL-first, minimal dependencies). Reasoning is in `./multi-db-strategy.md` and `./research-data-ops-best-practices.md` §3: this schema is "mostly shared DDL, occasionally per-backend", and goose's loose directory model fits that better than golang-migrate's stricter paired up/down files. golang-migrate is an equally defensible tool and I will not contest a swap if 03 already has tooling preferences — **nothing in §3–§6 below changes if the tool changes.** Atlas is the schema-as-state alternative worth knowing about; it is more than this take-home needs.

Version numbering: timestamped filenames during development (avoids collisions when two branches add a migration), converted to sequential with `goose fix` before anything is tagged. Not a decision 04 needs to act on — noted so nobody "fixes" the inconsistency later.

## 2. Layout

```
migrations/
  shared/     # applied to every backend, in order
  postgres/   # Postgres + CockroachDB only
  sqlite/     # currently empty, kept so its emptiness is deliberate rather than forgotten
```

`shared/` carries `auth_method`, `user_profile`, `user_credential` and the `auth_method` seed row. `postgres/` currently holds exactly one file — the `pg_trgm` GIN index on `user_profile.name`, which has no SQLite equivalent and is omitted there rather than faked. That parity gap is documented and deliberate (`./multi-db-strategy.md`); SQLite is the local/dev/demo backend, not a production peer.

**Mechanism 04 needs:** goose applies one directory per invocation, so a migration run is **two invocations** — `shared/` then the backend directory — and each needs its own version table (`goose -table goose_db_version_shared`, then `-table goose_db_version_<driver>`). One shared table across two directories would interleave version numbers from independent sequences and corrupt the history. Both invocations must succeed for the run to count as successful.

**One open item, flagged rather than buried:** whether a single `shared/` DDL series survives all three backends depends on type names (`UUID`, `BOOLEAN`, `TIMESTAMPTZ`) passing through SQLite's type-affinity rules without surprises — SQLite assigns NUMERIC affinity to type names it does not recognise, which is benign for text UUIDs but is exactly the kind of thing that is benign until it isn't. This is under review in S1. **If the review forces a split, the fallback is per-backend `CREATE TABLE` files and `shared/` reduced to seed data — the three-directory layout, the two-invocation mechanism, and everything in §3–§6 are unchanged.** 04 can build against this now; the open item cannot invalidate the infra work.

## 3. Requirements on 04 — the part I actually need decided

**R1. Migrations run as a separate step that completes before any service replica starts.** Not on service boot. Two replicas booting together would both attempt to migrate, and see R2 for why the database will not save you from that. A dedicated job/init step that must exit zero before the service rolls.

**R2. Exactly one migration runner at a time, enforced by the platform — not by the database.** This is the requirement I most want 04 to read carefully, because the obvious mechanism does not work here. `pg_advisory_lock` is the standard answer on PostgreSQL, but on CockroachDB **session-scoped advisory locks error out, and the `pg_try_advisory_lock` variants silently no-op** — a try-lock returns as though it succeeded and lets a second runner proceed (sourced in `./research-cockroachdb-postgres-semantics.md` §5; only transaction-scoped `pg_advisory_xact_lock` works there). On SQLite the concept is meaningless. A silent false success is a worse failure than an error. Since this project's whole premise is one schema across all three backends, a locking strategy that only works on one of them is not a strategy. So mutual exclusion has to come from the orchestration layer — a job with parallelism 1, a pipeline stage that cannot run concurrently with itself, whatever 04's platform offers. If 04 concludes the platform cannot guarantee this, tell me and I will treat it as a real constraint on the design rather than a detail.

**R3. The service fails closed when the schema is behind.** On startup the service compares the applied version against the version it was built against and refuses to serve if it is behind, rather than serving traffic against a schema missing a column it will query. The requirement is mine; whether the expected version is embedded at build time or derived from the migration files shipped in the image is 03's and 04's call.

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

## 6. What changes if the abstraction ruling changes

S3 in `./PLAN.md` is an open three-hats run on the multi-DB abstraction, including codegen (sqlc) as a real option that has been name-checked but never argued here. For honesty about what is settled: if codegen wins, the DAO's query layer changes and 03's work changes with it — **but migrations do not.** sqlc generates Go from SQL; it does not manage schema versions. The layout, the two-invocation mechanism, R1–R5, and the seed-data rule survive that ruling intact. Nothing in this document is waiting on S3.

---

*AI tooling note: drafted by Claude Opus 5 in the 05-data-ops track-lead session, from this track's own research doc and strategy doc, then put through a single devil's-advocate objection turn (Beatriz Achterberg, Spreadsheet archetype, sonnet) before hand-off. Per `../CASTING.md` §5, the objection and its disposition are recorded in `./decisions/`.*
