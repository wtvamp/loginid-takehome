# Decision: objection turn on the migration approach

Pattern: **rotating devil's advocate** (`../../CASTING.md` §5 — the cheapest pattern in the catalog, meant to be run often). Objection seat: Beatriz Achterberg (Spreadsheet). Decision owner: Priya Nandakumar. Story: S4 in `../PLAN.md`. Subject: `../migration-approach.md`, the 05→04 hand-off.

Six objections raised, **six accepted**. One of them overturned the reasoning behind the document's central requirement.

## 1. R2's conclusion no longer followed from its own corrected premise — accepted

The strongest objection, and the one I would have shipped without.

R2 requires that exactly one migration runner execute at a time, enforced by the orchestration platform rather than the database. The original argument was: "a locking strategy that only works on one of three backends is not a strategy." That was true of the premise as first written — that CockroachDB has no advisory locks at all. **But I had already corrected that premise**, in `../research-cockroachdb-postgres-semantics.md` §5: transaction-scoped `pg_advisory_xact_lock` *does* work on CockroachDB. Session-scoped locks error, and the `pg_try_*` variants silently no-op.

The objection: with the corrected facts, a transaction-scoped lock works on **both** backends that can actually have replicas — SQLite being a single-file local backend where two concurrent migration runners is not a scenario that exists. I had corrected the premise and left the conclusion resting on the old one, which is a worse failure than never correcting it: the document read as though it were reasoning from evidence it had already superseded.

**Ruling: the requirement stands; the reasoning is replaced.** A transaction-scoped advisory lock is released when its transaction ends, and goose applies each migration file in its own transaction — so the lock would be released *between files*, letting a second runner interleave mid-run. That is exactly the failure it was meant to prevent. Session-scoped locks would span the whole run, and those are the ones CockroachDB does not support. **The database can offer the right scope or the right engine coverage, never both.**

The objection anticipated that a good reason existed and said so explicitly — "*Closes if:* you show the transaction-scoped lock is impractical for a different reason… That is a good reason. It is not the reason written down." That is the distinction between an objection and a complaint, and it is why this seat is worth one turn per decision.

**This is the third time in this track that a hire has left a conclusion standing and destroyed the reasoning under it** (after the S3 dialect-branch count and the S2 error-translation rule). Three times is a pattern rather than three incidents: conclusions here have generally been right, and the arguments offered for them have generally been the first plausible one to hand rather than the true one.

## 2. The cheapest backstop was never examined — accepted

The document asserted the database could not help, without examining the one database-level mechanism goose itself creates: its version table, the single artifact all three backends share. A unique constraint on `version_id` would make a second concurrent runner fail on insert — portable, free, no platform feature required.

**Ruling: accepted, and recorded with its gap visible.** Whether goose's default version table already carries that constraint is **unverified**, and the document now says so rather than asserting either way. The actionable form does not depend on the answer: add a unique index on `version_id` in the first migration, which costs nothing if goose already does it. It is a backstop, not a replacement for R2 — it converts a silent interleave into a loud failure, which is the property worth buying.

Leaving this as an explicit "unverified" rather than resolving it was deliberate. The objection's own framing — an empty cell is a question, not a gap to paper over — applies to the author as much as to the tables she wrote.

## 3. The tool-independence claim had a counterexample in its own document — accepted

`migration-approach.md` claimed "nothing in §3–§6 changes if the tool changes." But §2's two-invocation mechanism uses goose's `-table` flag to keep two version sequences apart; swapping to golang-migrate means re-deriving it. Minor in effect, but it is the claim's only load-bearing use. **Ruling: accepted; the exception is now named inline.** An overclaim in a document whose entire argument is about not overclaiming would have been a poor advertisement for the rest of it.

## 4. R3 checked one version number where §2 creates two — accepted

Fail-closed said the service compares "the applied version" against the version it was built against. §2 creates **two** independent sequences — `shared/` and the backend directory. As written, a service could pass the check with `shared/` current and the `pg_trgm` index unapplied: a passing check that proves nothing, which is worse than no check.

**Ruling: accepted.** R3 now requires comparing both, and knowing which backend directory applies to the engine it is connected to.

## 5. `goose fix` had no stated cutoff — accepted

The document said to renumber "before anything is tagged." The real constraint is earlier and different: renumbering after migrations have been applied anywhere persistent rewrites history the version table already records. **Ruling: accepted** — it is a pre-first-application step, not a tidy-up before tagging.

## 6. A gap the objector's own other deliverable created — accepted

`deletion_log`, introduced by the retention mechanism in `../pii-governance.md` (S5), is schema, and §2's `shared/` contents list did not include it. Caught by the hire who had written the table that created the gap, in the course of attacking a different document.

**Ruling: accepted;** `shared/` now lists it. Worth recording because it is the argument for running the objection seat *after* the sibling deliverable rather than in parallel with it — a cross-deliverable inconsistency is invisible to anyone holding only one of the two.

## Also recorded: an unmodelled area, flagged by the objector against her own blind spot

Not an objection to this document, but raised alongside it and worth keeping: **subject-deletion requests reach `user_profile` and its cascade, and do not reach derived data** — logs, metrics, connector traces that may have incidentally captured PII in transit. That data has no `source` column, no retention clock, and often no key to select on.

Her framing, which I am adopting verbatim into `../pii-governance.md`: *"I have not modelled it. I would rather say so than let its absence read as a decision that it doesn't exist."* It sits across 02 (never-log list) and 04 (log retention), and the cheapest control is upstream of this track entirely — if PII never enters a log there is nothing to delete from it.

## Outcome

`../migration-approach.md` is **final** and has gone to 04.

---

*Pattern footer (`../../CASTING.md` §5 ⟨02⟩). Seat: objection-writer Beatriz Achterberg — sonnet, 1 turn (delivered alongside the S5 Table B tail). Rulings and revisions: Priya Nandakumar — Claude Opus 5. Hire turns consumed: 1. Objections raised: 6. Accepted: 6. Declined: 0. Cheapest pattern in the catalog; it overturned the reasoning behind the document's central requirement for one turn.*
