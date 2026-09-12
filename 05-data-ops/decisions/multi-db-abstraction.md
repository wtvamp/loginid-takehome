# Decision: the multi-database abstraction shape

Pattern: **three hats** (`../../CASTING.md` §5). Decision owner: Priya Nandakumar, 05-data-ops. Story: S3 in `../PLAN.md`.

**Question.** Per-driver implementations behind one interface (what `../multi-db-strategy.md` already recommends) vs. a single implementation with dialect branches vs. codegen (sqlc).

**Why it was run at all.** The existing recommendation was reached by one agent reading sources, and codegen had been name-checked in this track's documents without ever being argued. A recommendation nobody has attacked is an assumption wearing a conclusion's clothes. The three hats wrote in parallel and none saw another's position before this synthesis.

## Ruling

**Confirm per-driver implementations behind one interface — with the justification corrected, and conditional on a conformance suite.**

Three things changed in reaching it, and the second is the one that matters most:

1. **Codegen is not a competitor and the three-way framing was wrong.** sqlc generates a `Querier` per engine from per-engine SQL; those Queriers are distinct types satisfying no common interface until someone hand-writes one. Codegen therefore *fills* the boxes that per-driver implementations draw — it is orthogonal to the structural question, not an answer to it. The real choice was always two-way. **sqlc is 03's implementation preference to take or leave**, with one caveat from the optimist seat worth passing on: dynamic optional filters are sqlc's weakest case, and `Search()` — the flagship query of this contract — is exactly that shape. Generated CRUD plus one hand-written query per package buys type safety on the easy half and costs a toolchain.

2. **The document's stated reason for rejecting dialect branches is wrong, and is corrected here.** `../multi-db-strategy.md` rejects them on "`if backend == "sqlite"` scattered through query construction." The pragmatist seat counted: of the six concerns in that document's own dialect table, exactly **two** survive as real conditionals — app-side PK generation for SQLite, and fuzzy name search (`pg_trgm` vs `LIKE`). Placeholders are the driver's problem, upsert syntax is identical across all three, JSON is unused, booleans map at the boundary. "Scattered" describes a schema we do not have. It is two `if`s.

   So the two options cost roughly the same, and **the choice between them is made on reviewability, not on cost.** That is a legitimate tiebreak — two parallel files a reviewer can diff read better than one file with two apologetic conditionals, and this submission is graded on design reasoning — but it must be stated as what it is. A reviewer who checks the count finds the discrepancy in ten seconds; a document that pre-empts that reads as honest, one that doesn't reads as padded. Ruling for the cheaper-to-review option while *claiming* it is the cheaper option would be the kind of small dishonesty this whole exercise is supposed to catch.

3. **The real risk of the chosen option is silent behavioural divergence, not duplicated code.** The interface guarantees both packages compile. Nothing guarantees they behave alike, and the failure mode is invisible: no error, just different results. The pessimist seat produced a concrete, previously undocumented instance — see below. **The mitigation is not code review. It is one conformance suite, run against every backend.** Without it, per-driver implementations are the *most* expensive option rather than the cheapest, and the ruling is conditional on it existing.

## Consequent corrections to `../multi-db-strategy.md`

Both are errors in that document found by this run, recorded here so the fix is traceable rather than silent:

- **The SQLite search parity gap is a correctness gap, not a performance gap.** The document says `Search()` "behaves identically from the caller's side" on SQLite, with only a fallback to `LIKE '%term%'`. That is false. SQLite's `LIKE` is case-insensitive for ASCII by default; PostgreSQL's is case-sensitive. The same `Search("smith")` returns **different row sets** per backend, with no error anywhere. The word "identically" was hiding a behavioural difference. S1 must state the gap as one of result semantics, and S2 must decide whether the two backends are brought into agreement or the divergence is documented as accepted.
- **The rejection reasoning for dialect branches**, per §2 of the ruling above.

## Requirement flowing to 03

**A single conformance suite, written once against the interface, executed against every backend implementation.** Not per-package unit tests — the same assertions run twice, so that a behavioural difference fails a test instead of reaching a user. The `LIKE` case-sensitivity divergence above is the worked example of what it is for, and it is the kind of defect that cannot be caught by reading either implementation in isolation, because each is correct on its own terms.

Mechanism is Renata's (`../../03-engineering-delivery/`); the requirement is this track's, because it is the condition under which this ruling holds. Carried into S1 as a named out-of-scope-for-redesign item.

## Out of scope for this ruling

Whether one `postgres` package can honestly serve both PostgreSQL and CockroachDB is a **separate decision**, deliberately excluded from this run so three hats would not argue about evidence none of them had. It was raised by the Systems Cartographer seat at check-in, researched under `../../CASTING.md` §3 by the lead, and answered in `../research-cockroachdb-postgres-semantics.md`: yes, but only with an engine-aware retry-and-error seam. That finding constrains what goes *inside* the `postgres` package; it does not disturb the structural split ruled on here.

## Positions as written

Preserved rather than summarized away, per the pattern protocol. All three are reproduced in full in the hires' returned results; the load-bearing content of each:

- **Optimist (Anders Vogel, Systems Cartographer).** Confirmed per-driver by argument. The boundary that matters runs between the service layer and anything that knows a dialect; dialect branches move engine-awareness from one place (startup wiring) into every query that needs one, making the SQLite path "negative space inside the Postgres path." Established the codegen-is-orthogonal point that reframed the whole run. Noted unprompted that his case rests on the two engines that exist and needs no third — his stated blind spot, checked before it had to be named.
- **Pessimist (Yusuf Karadag, Detail Hawk, substituting for the Veteran seat).** Each option's failure stated as the day it shows up in production. Found the `LIKE` case-sensitivity divergence. Argued that dialect branches fail on *test coverage*, not tidiness — every conditional is a branch CI runs on exactly one backend — which is a better objection than the document's own. Flagged the CockroachDB question as an open question rather than a finding, as instructed.
- **Pragmatist (Beatriz Achterberg, Spreadsheet).** Counted files and branch points before forming an opinion. Produced the two-conditionals count that overturned the document's rejection reasoning, and named the consequence plainly: the choice is aesthetics and reviewability, so say so. Rejected codegen as the only option with a genuinely different cost profile — a dependency, a generate step, two config files, two query sets, and generated code in the repo, for a payoff (compile-time query verification) that is invisible in a submission nobody runs.

Where the seats disagreed: the optimist treated per-driver as clearly cheapest; the pragmatist showed it is not cheaper, only more reviewable. **The pragmatist was right on the facts and the optimist right on the conclusion** — which is the outcome that justifies having run the pattern, since a single agent would have produced the right answer with the wrong reason and nobody would have checked.

---

*Pattern footer (`../../CASTING.md` §5 ⟨02⟩). Seats: optimist Anders Vogel — sonnet; pessimist Yusuf Karadag — sonnet; pragmatist Beatriz Achterberg — sonnet; synthesis and ruling Priya Nandakumar — Claude Opus 5. Hire turns consumed: 3 (one per seat, parallel). Supporting deep-research pass: 1 sonnet research subagent, authorized and framed by the lead, output at `../research-cockroachdb-postgres-semantics.md`. Total: 4 hire-equivalent turns.*
