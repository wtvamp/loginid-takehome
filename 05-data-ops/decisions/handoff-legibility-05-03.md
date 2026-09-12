# Decision: cold-read legibility pass on the 05→03 contract

Pattern: **newcomer's question** (`../../CASTING.md` §5). Decision owner: Priya Nandakumar, 05-data-ops. Story: S6 in `../PLAN.md`. Subject: the Go-shaped contract in `../multi-db-strategy.md`.

## Why this was run, and the deliberate restriction

`../../02-ai-security-architecture/planning-approach.md` §2 records a standard this track asked the project to adopt: **a hand-off is implementable only if the receiving track can act on it without re-deriving the sender's reasoning.** Having asked for that standard, failing to test my own output against it would have been the worst available outcome.

The Newcomer archetype was at its org-wide cap, so the seat was played by Saoirse Byrne (Storyteller, this track's required temperament opposite). To make her a usable instrument she was **instructed at spawn not to read `multi-db-strategy.md`, `pii-governance.md`, or `research-data-ops-best-practices.md`**, and she sat out the S2 and S3 debates the other three hires participated in. That cost this track a seat in two debates. It was worth it.

She declared one contamination herself, unprompted: `../PLAN.md` told her what the contract *intended* to contain. She reported against the text rather than the intent and flagged the gap where the two diverged — which is how blocking item 4 below was found.

A second, independent cold read of this track's other hand-off (`../migration-approach.md`) was run in parallel by Wesley Okonkwo, on loan from 04. His findings are recorded at the end.

## Outcome

**14 blocking items. All 14 accepted. All 14 fixed.** Plus three findings in the tail, all three adopted.

No item was declined, which is a fact about the quality of the read rather than about the document: six of the fourteen identified something the contract *asserted* but did not *enforce* or *define* — the same failure class the S2 ruling had already named, recurring in the very document that adopted the rule against it.

### The four that would have become defects in 03's code

1. **`Update`: full replace or partial patch — undefined.** Nothing in the contract answered whether passing a profile with `Phone` nil NULLs the stored column or leaves it. Both readings are defensible; the wrong guess silently destroys data. *Ruled: full replacement, stated in bold. Patch semantics are read-modify-write at the caller.* She called this the single most consequential gap and was right.

2. **The error-translation rule was self-contradicting and half-unimplementable.** The contract said "translation is by SQLSTATE code only — never by constraint names," then immediately asked for different sentinels for `23505` depending on which column collided, which the code alone cannot say. Worse: **SQLite has no SQLSTATE at all**, so the rule was unimplementable across half the codebase. *Ruled: match structured codes per engine (SQLSTATE on PG/CRDB, extended result codes on SQLite), disambiguated by constraint names **we define in our own DDL** — our artifact, stable across engines, unlike an engine-generated message. Full mapping table added; every constraint in §5 now explicitly named.*

3. **`Upsert` could not serve the caller it was justified with.** The contract introduced `Upsert` for the IDP hydration path, which "does not know whether the row exists" — but `Upsert` conflicts on `id`, and a caller that doesn't know the row exists doesn't know its `id` either. There is no unique key on phone or name. *Ruled: the fix is deletion of the claim, not explanation. `Upsert` is for callers that already hold an `id`; resolving an `/identity` payload to an existing profile is explicitly the connector's problem.*

4. **No sentinel for write-side validation.** `ErrInvalidQuery` protected reads; nothing protected writes, so the commonest caller mistake — violating a CHECK constraint on `source`, `name`, `phone` or `country` — reached the service layer as a raw, engine-specific driver error. *Ruled: `ErrInvalidArgument` added, with `23514` / `SQLITE_CONSTRAINT_CHECK` mapped to it.*

### The item that applied this document's own standard back at it

5. **`ErrInvalidMethod` is documented as "unknown *or inactive*", and the FK catches only *unknown*.** `is_active = false` had no enforcement site, in a numbered list whose entire premise is that every guarantee names one. *Ruled: added as §6 item 10, enforcement site named as DAO-layer validation plus a conformance test, and flagged as one of the two weakest sites in the contract rather than quietly patched.* The standard catching its own author is the strongest evidence available that it works.

### The remaining nine, all accepted

`Create` and the `ID` field contradicted each other (§2 said the DAO generates it, §3 returned `ErrAlreadyExists` "if the ID is taken") — ruled: caller passes `""`, non-empty is `ErrInvalidArgument`. Ownership of `CreatedAt`/`UpdatedAt` unstated — ruled: DAO-owned, ignored on input. `total` never defined in the contract despite `PLAN.md` naming it as something S2 had to settle — the intent-versus-text gap; now defined as all matching rows ignoring limit/offset, and as a magnitude rather than a promise of reachability. `Phone` match semantics contradicted between the struct comment ("exact") and the index table ("exact/prefix") — ruled exact. All-nil `ProfileQuery` legality unstated — ruled legal. `Delete` on a missing row unstated — ruled `ErrNotFound`. `GetByUsername` case-folding ownership unstated — ruled DAO-side. The SQLite `GLOB` phone constraint was left as "a GLOB equivalent" — she correctly noted that inventing it is a design decision, not an implementation detail; it is now given in full.

### Surprise-versus-warning audit

She was asked, for each of the contract's three counter-intuitive behaviours, whether a reader meets the surprise before or after the sentence warning them:

| Behaviour | Verdict | Action |
|---|---|---|
| `Update` does not create | **Warned first**, immediately under the method block | none |
| Cascade delete profile→credential | **Surprise ~140 lines before its explanation** at §6.2 | pointer added at the method list |
| SQLite `Search` parity | **No warning at all** — the only trace was "Postgres + CockroachDB only" in the index table, which a reader cannot turn into behaviour | new subsection: same results, same order, no index, full scan; non-ASCII folding the named residual |

### Tail findings, all adopted

- **The retry seam ruled *that* writes retry, never *where* the wrapper sits.** Three implementers would produce three faithful, different codebases. Now settled: an unexported helper inside the `postgres` package; **no shim at all** in `sqlite` (a no-op would imply a shared abstraction that does not exist); not in the composite, which would wrap reads and push engine-specific behaviour above the interface boundary.
- **The `"postgres"` / `"cockroachdb"` driver-string split asserted the seam "needs to know which engine" without saying how it finds out**, given both select the same package. Now: an unexported `engine` field on the implementation struct, which is the entire reason the two strings are distinct.
- **The address rows collapsed four columns into one `CHECK (x ...)` plus three "as above"** — requiring the reader to substitute the column name four times, immediately above the row where the SQLite variant was genuinely missing. Her point: the reader is trained to fill in a blank just before reaching a blank they cannot fill. All four written out and named.
- **Organized by artifact; implemented by verb.** Four of the fourteen blockers existed only because no single place assembled a method's full story. She raised this as a question rather than a rewrite request and said explicitly that "no" was a fine answer. *Adopted:* §3a, a per-method table — who supplies the ID, who stamps timestamps, replace-or-patch, errors returned — which closes those four by construction rather than one prose sentence at a time.
- **On "enforcement site" as vocabulary**, she volunteered the opposite of a finding: it arrives self-defining and should *not* get a glossary entry. She also noted that §6's admission of its own weakest link reads as honesty rather than hedging only because the standard was stated first — the ordering is load-bearing.

### Jargon undefined on first use

`gin_trgm_ops`/GIN, `crdb.ExecuteTx`, SQLSTATE, `COLLATE "C"`, `40001`/`23505`/`23503`, SERIALIZABLE vs. READ COMMITTED — all now defined in a vocabulary block. Bare `S1`/`S2`/`S3` story shorthand used without expansion: **the second reader in a row to catch this**, after Wesley found it in `migration-approach.md`, which makes it a habit of the author's rather than an oversight. Fixed as a class in both documents.

## Parallel cold read: `migration-approach.md` (Wesley Okonkwo, 04, on loan)

Six of seven findings were undefined-on-first-use vocabulary: `goose`, `goose fix`, SQLite "type affinity", "advisory lock", and `pg_trgm`. All now defined inline.

One was not cosmetic. He could not tell whether `pg_trgm` was background or actionable — and it is actionable: it is precisely why the migration runner needs `CREATE EXTENSION` privileges on PostgreSQL and none on CockroachDB. Theo confirmed the consequence downstream: he had not broken the migration-runner credential out as its own row in `secrets-delivery.md`, and has now separated it from the runtime DML-only credential. **A glossary gap in this track's document had propagated into a missing row in another track's secrets inventory.**

He also flagged, against his own stated blind spot (mistaking a deliberate open item for a gap), that R2's "tell me if the platform cannot guarantee single-runner exclusion" read as a genuinely open question handed to 04. It was, and it now says so in bold.

## What this pass establishes

The hand-off standard this track asked the project to adopt is testable, and the test fails on a document written by the person who proposed the standard. That is the useful result. A contract reviewed only by people who watched it being written is reviewed for correctness, not for legibility, and those are different properties — every one of the fourteen items was invisible to four readers who already knew what the document meant.

**Recommendation for any future run of this seat: keep the reader cold, and accept the cost.** Saoirse's own closing assessment is the argument: she cannot say which of the fourteen she would still have found had she read the debates first, and that uncertainty is exactly why the restriction is worth its price.

---

*Pattern footer (`../../CASTING.md` §5 ⟨02⟩). Seats: cold reader Saoirse Byrne — sonnet, 2 turns (read, tail); parallel cold reader Wesley Okonkwo (04, on loan, ≤4 turns authorized) — sonnet, 1 turn, relayed by the 04 lead; revisions and rulings Priya Nandakumar — Claude Opus 5. Hire turns consumed: 3. Blocking items raised: 14. Accepted: 14. Declined: 0. Tail findings: 3, all adopted.*
