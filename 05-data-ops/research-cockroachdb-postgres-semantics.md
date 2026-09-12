# Research: CockroachDB vs PostgreSQL — Transaction and Error Semantics

Owner: 05-data-ops. Authorized and run by the track lead (Priya Nandakumar) per `../CASTING.md` §3 "Deep research — leads only"; executed as a single scoped sonnet research pass, synthesized here.

**Why this exists.** `./multi-db-strategy.md` claims one Go `postgres` package can serve both PostgreSQL and CockroachDB. Anders Vogel (Systems Cartographer) flagged in his first check-in that the doc defends that claim on wire protocol and SQL surface only — "the cheap half of the argument" — and that the interaction surface worth mapping is transaction and error semantics, where the two engines are least alike. He was right, and the existing claim does not survive unqualified. This pass exists to replace an assumption with evidence before it reaches 03 as a contract.

Every substantive claim carries a source URL. Vendor pages are labeled. The "could not verify" list at the end is deliberate and is not padding.

## Verdict

**One Go package can serve both engines — but not as a thin PostgreSQL implementation pointed at a second connection string.** It must carry an explicit engine-aware retry-and-error-translation seam. The divergence is narrow and nameable rather than architectural, which is why the two-package split in `./multi-db-strategy.md` survives; what does not survive is the sentence implying the two engines are interchangeable behind it.

## 1. Retryable transaction errors — the material divergence

CockroachDB defaults to SERIALIZABLE and returns SQLSTATE `40001` on serialization conflicts. PostgreSQL at its default READ COMMITTED **never** emits `40001`; it only does so at REPEATABLE READ or SERIALIZABLE ([PostgreSQL, Transaction Isolation](https://www.postgresql.org/docs/current/transaction-iso.html); [PostgreSQL, Serialization Failure Handling](https://www.postgresql.org/docs/current/mvcc-serialization-failure-handling.html)).

CockroachDB retries internally where it can, but there is a hard limit: once a multi-statement transaction spans more than one round trip, or once results have begun streaming to the client (or exceed `sql.defaults.results_buffer.size`), server-side retry becomes impossible and **the client receives `40001` and must retry itself** ([Cockroach Labs, Transaction Retry Error Reference](https://docs.cockroachlabs.com/docs/stable/transaction-retry-error-reference) — vendor documentation, but this is the vendor documenting a requirement on its own clients, which is the strongest form this kind of claim takes).

Cockroach Labs prescribes a client-side `SAVEPOINT cockroach_restart` retry loop ([Advanced Client-Side Transaction Retries](https://docs.cockroachlabs.com/docs/stable/advanced-client-side-transaction-retries)) and ships `crdb.ExecuteTx` to wrap it — 50 retries by default, configurable ([pkg.go.dev, cockroachdb/cockroach-go/crdb](https://pkg.go.dev/github.com/cockroachdb/cockroach-go/crdb)).

**The load-bearing inference:** `crdb.ExecuteTx` exists *because* plain `database/sql` is insufficient for CockroachDB under SERIALIZABLE. A default-configuration PostgreSQL DAO needs no equivalent. A shared implementation that omits the wrapper is not "compatible with both" — it is correct on PostgreSQL and intermittently wrong on CockroachDB under contention, which is the worst failure shape available: invisible in development, load-dependent in production.

## 2. Isolation levels

| | PostgreSQL | CockroachDB |
|---|---|---|
| Supported | READ COMMITTED, REPEATABLE READ, SERIALIZABLE | READ COMMITTED, SERIALIZABLE |
| Default | READ COMMITTED | SERIALIZABLE |

CockroachDB's READ COMMITTED has been GA since 24.1 ([Read Committed Transactions](https://docs.cockroachlabs.com/docs/v26.2/read-committed)). Running the CockroachDB backend at READ COMMITTED does change the answer to §1: under it CockroachDB "never returns RETRY_SERIALIZABLE errors," and the retry loop stops being mandatory for ordinary contention — though transactions can still abort on deadlocks, constraint violations, and rare streaming cases.

**A trap worth naming:** Cockroach's READ COMMITTED is documented as *stronger* than PostgreSQL's — it prevents anomalies within a single statement. So even when the isolation level name matches, the semantics do not. Matching the name is not matching the behaviour, which is the same error the original compatibility claim made one level up.

## 3. Constraint-violation error codes — aligned, with a sharp edge

Both engines use `23505` (unique_violation) and `23503` (foreign_key_violation); CockroachDB implements the PostgreSQL error-code table (`pkg/sql/pgwire/pgcode/codes.go` in cockroachdb/cockroach).

But [cockroachdb/cockroach#42858](https://github.com/cockroachdb/cockroach/issues/42858) documents a real historical inconsistency — a duplicate FK *constraint definition* returned `23503` in one CockroachDB version, `42830` in another, against `42710` on PostgreSQL. Closed by PR #43210, but it establishes that constraint-related SQLSTATEs have not always agreed across versions. [cockroachdb/cockroach#16196](https://github.com/cockroachdb/cockroach/issues/16196) records divergence in FK-violation error reporting. Error *text* and structure are not guaranteed portable either: PostgreSQL embeds the constraint name and a `DETAIL:` line naming the offending key.

**Direct consequence for the S1 contract, and the reason this section exists:** the DAO translates driver errors into the shared sentinel set (`ErrDuplicateUsername`, `ErrInvalidMethod`) **by SQLSTATE code only — never by matching message text or constraint-name substrings**, and the CockroachDB target version is pinned. String-matching an error message is fragile against one engine and reckless against two.

## 4. Go drivers

`pgx` and `lib/pq` work against CockroachDB largely unchanged for basic CRUD, since CockroachDB implements the PostgreSQL wire protocol — this is [CockroachDB's own compatibility page](https://docs.cockroachlabs.com/docs/stable/postgresql-compatibility), **vendor-asserted and cited here for what it claims, not as proof it holds**. That same page states plainly that CockroachDB is not a drop-in PostgreSQL replacement.

The existence of `cockroachdb/cockroach-go` (and `crdbpgx` for pgx-native pools) is the more informative evidence: a vendor does not ship a transaction wrapper for its own database unless the standard client path is insufficient.

## 5. Other divergences that leak into a shared implementation

All from CockroachDB's [PostgreSQL Compatibility](https://docs.cockroachlabs.com/docs/stable/postgresql-compatibility) and [Known Limitations](https://www.cockroachlabs.com/docs/stable/known-limitations) pages (vendor-documented):

- **Advisory locks.** Session-scoped `pg_advisory_lock` errors out on CockroachDB; only transaction-scoped `pg_advisory_xact_lock` works. Worse, `pg_try_advisory_lock` and its unlock counterpart **silently no-op** rather than erroring. A design relying on advisory locks for mutual exclusion fails *silently* on CockroachDB — see §6, because this corrects a claim in this track's own hand-off to 04.
- **Savepoints.** `ROLLBACK TO SAVEPOINT` does not fully match PostgreSQL semantics for cursors defined before the savepoint.
- **`SELECT ... FOR UPDATE`.** Not usable with cursors on CockroachDB; less optimized under READ COMMITTED than SERIALIZABLE.
- **`ON CONFLICT DO UPDATE` under contention.** Falls inside the §1 retry framework — a contended upsert under CockroachDB SERIALIZABLE can itself throw `40001`. This one matters disproportionately here: `Update()` in our contract *is* a portable upsert, so our single most-used write path is the one exposed to retryable errors.
- **Transaction priorities.** `SET TRANSACTION PRIORITY` is CockroachDB-only with no PostgreSQL equivalent — a shared interface must not expose it uniformly.

## 6. Correction to this track's own `migration-approach.md`

`./migration-approach.md` R2 stated that "CockroachDB does not implement advisory locks." That is too strong and is corrected: transaction-scoped advisory locks (`pg_advisory_xact_lock`) do work; session-scoped ones error; and the `pg_try_*` variants silently no-op.

**The conclusion R2 draws is unchanged and is in fact strengthened.** A migration-runner lock built on `pg_try_advisory_lock` would not fail loudly on CockroachDB — it would return as though the lock were acquired and let two runners proceed. Single-runner mutual exclusion must come from 04's orchestration layer, not from the database. The requirement stands; only my reason for it was imprecise, and the corrected reason is worse than the one I gave.

## Could not verify

Stated as gaps rather than filled with inference:

- Whether the `#42858` SQLSTATE inconsistency extends to ordinary INSERT-time FK *data* violations (`23503`) on current CockroachDB versions — the closed issue documents a DDL-time case only.
- The exact current-version message/`DETAIL` structure diff between the engines for `23505`/`23503` — specifically whether CockroachDB populates `Detail` and `ConstraintName` on `pgconn.PgError` identically to PostgreSQL. **This gap is the direct reason §3 mandates SQLSTATE-only matching**; the rule holds regardless of how the gap resolves, which is why it did not need resolving before writing the contract.
- Whether `crdbpgx.ExecuteTx` has behaviourally diverged from the `database/sql` variant `crdb.ExecuteTx`.

## What this changes downstream

1. **S1 contract — error semantics.** Sentinel translation is by SQLSTATE only, never by message text. Stated as a requirement on 03, with this document as the citation.
2. **S1 contract — a new decision, not previously in scope.** Either every write goes through a `crdb.ExecuteTx`-style retry wrapper (near-zero cost on PostgreSQL, load-bearing on CockroachDB), or the CockroachDB backend is explicitly documented as running at READ COMMITTED to drop the requirement. One of the two must be chosen and written down; leaving it unstated is how the intermittent-failure mode gets shipped. **This is the decision this research pass was worth running for.**
3. **`./multi-db-strategy.md`.** The shared-package justification is amended from "wire protocol and SQL surface compatibility" to that plus the named seam. The two-package structural split is unaffected.
4. **`./migration-approach.md` R2.** Corrected per §6; conclusion unchanged.

*AI tooling note: scoped research question authorized and framed by the 05 lead (Claude Opus 5) after a hire's flag, executed by one Claude Sonnet 5 research subagent with web search, synthesized and fact-checked against the cited primary sources by the lead. The hire who raised the question did not run the research — per `../CASTING.md` §3, deep research is leads-only.*
