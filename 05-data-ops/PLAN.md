# Track 05 — Data Ops: Planning Phase Plan

Lead: Priya Nandakumar (opus/high for this phase). Track directory: `05-data-ops/`. Org plan: `../PLAN.md`. Board row: `../PLANNING.md`. Rules, palette, patterns, hire procedure: `../CASTING.md`. Coordination approach: `../02-ai-security-architecture/planning-approach.md`.

## Context

All three of the assignment's questions sit on top of one schema and one DAO contract, and this track owns both. That is the whole reason 05 is on the first-wave critical path: 03 cannot write the DAO until the contract is stable, and 04 cannot plan migration execution until the migration approach is named.

The first-wave design work already exists here as prose — `research-data-ops-best-practices.md` (sourced research), `multi-db-strategy.md` (interface contract and schema shape), `pii-governance.md` (retention and non-conflation rules). What does not exist is the thing 03 actually needs. `planning-approach.md` records my own commitment on this and Marcus adopted it project-wide: a hand-off is implementable only if the receiver can act on it **without re-deriving the sender's reasoning**. `multi-db-strategy.md` today describes `Search(ctx, query) (results []UserProfile, total int, error)` in a sentence. It does not say what `query` is, which fields are nullable, what `Update` does when the row is gone, or whether `total` is the filtered count or the page count. Renata would have to invent all of that, and inventing it is a design decision that belongs to this track, not hers.

So the deliverable this phase is aimed at is narrow and specific: turn the prose contract into a Go-shaped contract — interface signatures, field-level schema with nullability and types, error semantics, and explicit out-of-scope notes telling 03 what it must not redesign. Everything else in this plan either feeds that artifact or unblocks 04.

The planning-phase rule holds throughout: this is a Go-*shaped* contract written inside a markdown file. No `.go`, no `.sql`, no `go.mod`, until Warren's explicit go-ahead. A fenced Go block inside `multi-db-strategy.md` is the design; a file the compiler would accept is code.

## 1. Deliverables and tasks

Six deliverables, each sized to one Jira Story under the track's Epic in project `LT`. Stories are listed in dependency order; S1 is the first Phase 3 deliverable and the one everything downstream waits on. Acceptance criterion for every story is the hand-off standard above.

### S1 — Go-shaped contract in `multi-db-strategy.md` (the critical-path deliverable)

Extend `multi-db-strategy.md` in place — it stays the single source of truth, no new parallel doc. Adds, as fenced Go-shaped blocks and tables rather than prose:

- **Interface signatures** for `ProfileRepository`, `CredentialRepository`, `AuthMethodRepository` — exact method names, parameter and return types, and the `ProfileQuery` / `ProfileSearchResult` types `Search` takes and returns (shape ruled in S2).
- **Error semantics**, which today are one sentence about `ErrNotFound`. Needs the full sentinel set and when each is returned: `ErrNotFound`, `ErrDuplicateUsername` (unique-constraint violation on `user_credential.username`, which both backends must translate out of their driver-specific error into the shared sentinel), `ErrInvalidMethod` (FK violation on `method_id`), and the rule that everything else is wrapped with `%w` and passed through. Also: does `Update` on a missing row return `ErrNotFound` or upsert-create? (My position going in: `Update` is a portable upsert per the existing doc, so it creates — which means the doc must say so loudly, because "Update creates" is exactly the surprise that produces a bug in 03.)
- **Field-level schema table per entity** — column, Go type, SQL type per backend family, nullable yes/no, constraint, and a one-line note. Nullability is where this contract is most likely to be wrong and most expensive to fix, so it gets its own reviewer (S3).
- **Context and cancellation**: every method takes `ctx` first and is expected to honour cancellation; no method holds a transaction across calls. If the service layer needs a multi-statement transaction, that is a follow-up on the contract, not something 03 improvises — stated explicitly.
- **Explicitly out of scope for 03** — a numbered list so there is no ambiguity about what 03 must not redesign: the PII/credential separation (governance control, `pii-governance.md`), the UUID-everywhere PK strategy, the `auth_method` lookup table rather than a native enum, the `ON CONFLICT DO UPDATE` upsert form, the documented SQLite fuzzy-search parity gap, and the two-package (`postgres` serving Postgres + CockroachDB, `sqlite` separate) split. Each with a one-line "why, and who to flag if it doesn't fit."
- **Alignment note to 03's published factory**: `../PLANNING.md` service boundaries names `dao.New(driver, dsn string) (dao.Repository, error)` — a single `Repository` return type, while this track specifies three interfaces. Resolve it in the contract (my expected resolution: `dao.Repository` is a small composite that exposes `Profiles()`, `Credentials()`, `Methods()`, keeping Renata's factory signature stable exactly as she declared it) rather than leaving two tracks each thinking the other will bend.

Consumes: S2 (query shape), S3 (nullability review), and the S4 ruling. Produces the 05→03 hand-off. Story closes when S6's cold read comes back clean.

> *Annotation, 2026-09-12 — the plan text above is left exactly as approved.* "the S4 ruling" is an error: S4 is the migration approach, which this contract never consumed; it should read "the S3 ruling". The stale reference propagated into `./backlog.md` and then into Jira as LT-19 blocking LT-16, and was caught by the scribe transcribing it literally. Corrected in `./backlog.md` Story 3 and in Jira (LT-16 depends on LT-14 and LT-15; the real link runs LT-16 → LT-19). Recorded in `../LOG.md` as a finding about the method rather than a typo: reviewers read documents, and nobody was scoped to read the dependency graph.

### S2 — `Search()` query shape decision

The one genuinely open design question in the contract, and the one with the most ways to be quietly wrong. Decides: the `ProfileQuery` field set and which fields are optional; match semantics per field (name partial/fuzzy, phone exact-or-prefix on the E.164-normalized value, region/country exact); how empty and multiple filters combine (AND, and whether a wholly empty query is a legal "list everything" or an error); pagination as limit/offset versus a keyset cursor, with a maximum page size; sort order and whether it is stable enough for offset pagination to be meaningful at all; and what `total` means — total matching rows, or rows on this page. Also settles how the documented SQLite parity gap surfaces to a caller: identical results with worse performance, or ranked-vs-unranked differences a caller could observe.

Pattern: **adversarial pair** (`../CASTING.md` §5 names this decision by name). Artifact: `decisions/search-query-shape.md`, folded into S1.

### S3 — Multi-DB abstraction ruling

`multi-db-strategy.md` already recommends two packages behind one interface and rejects dialect branching. That recommendation was reached by one agent reading sources. The catalog names this decision for **three hats**, and it is worth the three turns because it is the structural choice 03 builds the entire DAO around — codegen (sqlc) is a real third option that was never actually argued, only mentioned. Possible outcomes: the existing recommendation is confirmed (most likely, and the confirmation is then a documented ruling rather than an assumption), or codegen displaces hand-written SQL, which changes what S1's contract has to pin down.

Artifact: `decisions/multi-db-abstraction.md` with the three ≤300-word positions, my synthesis, and the pattern footer (model per seat, hire turns consumed) that 02 added to every pattern.

### S4 — Migration approach hand-off for 04

`../PLANNING.md` lists 04 as blocked on "05's migration tool/approach". `multi-db-strategy.md` has a migration section, but in Theo's terms it is a preference, not a mechanism. Produces `migration-approach.md` — a new file, because its receiver is 04, not 03, and folding an infra hand-off into the DAO contract would make both harder to read:

- Tool: goose (already reasoned in `multi-db-strategy.md`; golang-migrate acceptable if 03 has tooling preferences — one rotating-devil's-advocate objection turn, not a debate).
- Directory layout and naming for the shared series plus the named per-backend override set (currently exactly one override: the `pg_trgm` GIN index, omitted on SQLite).
- What 04 must provide: when migrations run relative to service start, whether the service fails closed if the schema is behind, migration-lock behaviour under multiple replicas, and the DB privileges the migration runner needs versus the runtime user.
- Seed data: the `auth_method` `"password"` row is a seeded row, not a migration constant — how it is applied and what makes it idempotent.
- Explicitly not owned here: where the databases run, how the pipeline invokes goose, environment topology. Named so 04 does not read a boundary into this doc that isn't there.

### S5 — Retention windows and the deletion-job contract

`pii-governance.md` states the policy shape and stops at "a product/legal decision this track doesn't own outright." That is true and it is also not actionable — 04 cannot build a retention sweep against it. This story puts numbers in a table, marked as **proposed defaults for a POC pending a product/legal ruling**, and specifies the mechanism the numbers plug into: what a sweep selects on (`source`, `updated_at`), whether deletion is hard delete or tombstone, what cascades to `user_credential` (the FK is already cascade-delete, so deleting a profile silently destroys its credentials — deliberate and worth stating), and what a subject-deletion request touches versus a bulk retention sweep. Separate proposed windows for `source = 'direct'` and `source = 'idp_cache'`, with the `idp_cache` window shorter and the reasoning stated.

Owner seat: the Spreadsheet hire, who will not accept "a reasonable window" as an answer. Artifact: `pii-governance.md` extended with a retention table; policy shape only, execution stays 04's.

### S6 — Cold-read legibility pass on the 05→03 contract

The catalog calls the **newcomer's question** pass the highest-ROI pattern in the project and names "every 05→03 hand-off" as its use. A hire who has not watched this contract being written reads S1 cold and lists what they could not act on without asking me. I fix every item or record why not. This is the acceptance test for S1 and for the hand-off standard I asked the project to adopt; failing to run it on my own output would be the most embarrassing available outcome.

Artifact: `decisions/handoff-legibility-05-03.md`. Runs after S1 drafts, gates S1 closing.

## 2. Hire roster — 4 hires

Matches the suggested cast in `../CASTING.md` §3 exactly. Variance check against the fourteen hires already registered: Systems Cartographer, Storyteller, Detail Hawk and Spreadsheet each currently sit at 1× org-wide, so all four land at 2× — at the cap, none over it, and none repeated within this team. Rule 2 (same archetype, different function) holds in every case, stated per hire below. Rule 3 (lead's temperament opposite) is Saoirse. Rule 5: this roster spans serious↔playful and cautious↔bold both. Model: **sonnet** for all four; none of these seats makes a foundational call alone — I hold those at opus, which is the entire reason this session was relaunched.

Two seats in my debate plan have no archetype left in the palette (Designated Skeptic and Newcomer are both at 2× org-wide). Rather than burn a hire on a near-duplicate, I substitute within this roster and say so explicitly in §3 — the pattern protocols are about the role played, not the label worn.

### Hire 1 — Anders Vogel · Systems Cartographer · serious, bold

Function: multi-DB abstraction — drafts the interface contract's structural shape and the Proposer seat on `Search()`. Distinct from Ines Dabrowski's Systems Cartographer function in 03 (Go module layout and service topology): Anders draws the boundary *inside* the DAO between interface and per-backend implementation; Ines draws boundaries between packages and binaries. Different artifacts, no overlap.

> Persona paragraph for `../CASTING.md` §4 (127 words):
>
> Calm and unusually direct for someone who thinks in diagrams; opens by drawing the boundary before anyone argues about what sits on either side of it. Decides by mapping the interaction surface first — what crosses this line, in which direction, how often — then choosing the abstraction that makes the smallest surface. Pushes back hard on any interface whose implementations must know about each other, and on abstractions justified by a backend nobody has committed to adding. Six years on a payments platform's storage layer where a leaky repository interface bled dialect assumptions into three services, then four years at a data-infrastructure vendor building connectors across five engines. Blind spot: abstracts past the concrete — he will design for the fourth backend before the second one works. Concedes cleanly, in writing, and moves on.

### Hire 2 — Saoirse Byrne · Storyteller · playful, bold (my temperament opposite, rule 3)

Function: consumer's-eye reader and cold-read seat — argues the contract from the position of whoever has to use it, and runs the newcomer's-question pass on S1. Distinct from Imogen Hale's Storyteller function in 01 (product framing and narrative thesis): Saoirse's subject is a schema and an interface, not a market; her output is "here is where this hand-off stops being followable," not a README thesis.

> Persona paragraph for `../CASTING.md` §4 (133 words):
>
> Warm, quick, and cheerfully unbothered by being the least formal person in a schema review; narrates a design as the journey of one record — created here, read there, deleted when. Decides by walking the path end to end and noticing where the story skips a step, which is usually where the bug is. Pushes back on documents that are correct but unfollowable, and on any governance rule stated so abstractly that nobody could tell whether they had broken it. Five years writing developer documentation and SDK guides at an API company, three as a solutions engineer who watched customers misread her own docs in real time. Reads SQL comfortably and writes Go badly enough to be an honest proxy for a confused implementer. Blind spot: charming beats correct — the clearer story is not always the true one. Disagrees by retelling it.

### Hire 3 — Yusuf Karadag · Detail Hawk · serious, cautious

Function: DDL and nullability review, plus the numbered-objection seat on `Search()`. Distinct from Oren Castellan's Detail Hawk function in 03 (Go code review — error semantics, off-by-one): Yusuf reviews the schema and contract *before* code exists; Oren reviews code after. `../CASTING.md` §2 names this exact pair as the allowed second use.

> Persona paragraph for `../CASTING.md` §4 (129 words):
>
> Quiet, exacting, and slower to speak than everyone else in the room because he is still reading the column list. Decides one field at a time: type, nullability, constraint, default, and what happens on the day the constraint is violated in production. Pushes back on any nullable column whose null has no stated meaning, on "we'll validate it in the application layer," and on error handling that collapses three distinct database failures into one generic message. Nine years as a database engineer at a healthcare data processor under audit conditions, then three on a platform team migrating a large schema across engines. Deep in Postgres constraint semantics and the places SQLite quietly disagrees. Blind spot: cannot see the forest — he will hold a release over a defaulted column. Objects in numbered lists and expects numbered answers.

### Hire 4 — Beatriz Achterberg · Spreadsheet · playful, cautious

Function: retention windows and the deletion-job contract (S5), plus the pragmatist hat in the three-hats run. Distinct from Desmond Okafor's Spreadsheet function in 01 (auditing evidence behind claims): Beatriz sets numbers that become policy rather than checking numbers someone else asserted.

> Persona paragraph for `../CASTING.md` §4 (131 words):
>
> Dry, genial, and faintly delighted when a question she was told was qualitative turns out to have a number hiding in it. Decides by building the smallest table that forces the choice into the open — one row per case, one column per thing that must be true — and refuses to move until every cell is filled or explicitly marked unknown. Pushes back on "reasonable," "appropriate," and "as needed" wherever those words stand in for a duration, and on retention policies with no named mechanism to enforce them. Seven years in data governance at an insurance group writing the retention schedules the auditors read, then three on a privacy-engineering team building deletion pipelines. Blind spot: she measures only what is measurable, and will quietly drop what resists a column. Disagrees by showing you the empty cell.

## 3. Debate plan

Patterns are `../CASTING.md` §5 protocols; every run ends in a file under `05-data-ops/decisions/` with the ⟨02⟩ footer recording model per seat and hire turns consumed.

**Three hats — the multi-DB abstraction (S3).** Three parallel ≤300-word positions on per-driver implementations vs. dialect branches vs. codegen (sqlc):
- *Optimist* — Anders: the case for the cleanest structural answer, codegen included.
- *Pessimist* — Yusuf, substituting for the Veteran seat (both Veterans are 03's; Yusuf's failure-mode instinct fits): where each option breaks at the boundary, generated-code review burden, dialect drift.
- *Pragmatist* — Beatriz: what each costs in files, review surface and time for a take-home whose SQLite backend is explicitly not a production peer.

I synthesize and rule. 3 hire turns + my synthesis. Justified because it is the structural choice 03 builds against and codegen has never actually been argued here, only name-checked.

**Adversarial pair — the `Search()` query shape (S2).** Anders proposes the `ProfileQuery` / result shape; Yusuf writes numbered objections, each ending with the "what would change my mind" line ⟨02⟩ requires; Anders answers every objection in writing and may decline to revise, but never silently; I rule. Yusuf holds the Designated Skeptic seat, which is at 2× org-wide and unavailable — his numbered-objection habit is the part of that archetype this pattern actually needs. 3 hire turns + my ruling.

**Newcomer's question — the 05→03 contract (S6).** Saoirse reads the drafted S1 cold and lists what she could not act on without asking me; I fix or record why not. The Newcomer archetype is at 2× org-wide (Wesley Okonkwo, 04, currently on loan to 01), so Saoirse plays the seat — with one contingency: if her pass comes back thin, I will ask team-lead for a short Wesley loan for a second cold read, since the value of this pattern comes from genuine unfamiliarity and a hire who has read this track's docs is only a partial proxy. 1–2 hire turns.

**Rotating devil's advocate — two single-turn objections.** Beatriz objects to the migration approach (S4) before it goes to 04; Saoirse objects to the retention windows (S5) before they go into `pii-governance.md`. Cheapest pattern in the catalog; the seat rotates so neither becomes "the negative one."

**Explicitly not debated — and why:**
- *DAO CRUD semantics beyond `Search`* — `Get`/`Create`/`Delete` are settled by the assignment and by the existing doc; the catalog names DAO CRUD as red/blue waste and it is three-hats waste too.
- *No red team / blue team in this track at all.* The catalog reserves it for the connector token lifecycle, which is 02's. Attack-tree work against this schema would duplicate `../02-ai-security-architecture/threat-model.md` and cross an ownership line.
- *The address field names* (`street_address`, `locality`, `region`, `postal_code`, `country`) — fixed by the assignment text. Never debate what the assignment fixes.
- *Separate `user_profile` / `user_credential` tables* — fixed by the assignment and reinforced as a governance control. Reopening it would be theatre.
- *UUID primary keys everywhere* — settled by sourced research (CockroachDB write-hotspot behaviour). Re-arguing a decision with a citation behind it is waste; it goes in S1's out-of-scope list instead.
- *goose vs. golang-migrate* — a preference between two adequate tools. One devil's-advocate objection turn, not a pattern.
- *Migration execution mechanics* — 04's, not mine. I state the requirement; Theo decides the mechanism.

Total planned hire turns: 3 (three hats) + 3 (adversarial pair) + 1–2 (cold read) + 2 (devil's advocate) + 1 (Beatriz drafting S5) ≈ 10–11.

## 4. Dependencies

**This track is first-wave and on the critical path** (`planning-approach.md` §2): 05 hard-blocks 03's DAO and partially blocks 04. Nothing hard-blocks 05.

**Provides:**
| To | Artifact | Unblocks |
|---|---|---|
| 03 | `multi-db-strategy.md` — Go-shaped contract (S1) | The DAO. Renata is already listed as blocking on it in `../PLANNING.md`. Delivered as "stable" — unlikely to be rewritten — not as polished, per `planning-approach.md` §2. |
| 04 | `migration-approach.md` (S4) | One of Theo's three named inputs. |
| 04 | `pii-governance.md` retention table (S5) | The retention-sweep mechanism 04 owns. |
| 02 | The reserved `secret` / `hash_algo` / `hash_cost` columns, unchanged in S1 | Marcus's credential-hashing design already builds on them; my job is not to move them. |

**Consumes (all soft — none blocks the start of any story):**
| From | What | When needed |
|---|---|---|
| 01 | `api-connector-design-rationale.md` — designated source of truth for field and endpoint shape | Read before S1 closes, to confirm the `user_profile` columns still mirror `/identity` exactly. Soft dependency per `planning-approach.md` §2. |
| 02 | Confirmation that the credential-storage column set is still sufficient | Before S1 closes. Already coordinated once — the `method` lookup-table decision is the worked two-party example in `planning-approach.md` §2. |
| 03 | Service boundaries in `../PLANNING.md` — already published | Consumed now. The `dao.New(driver, dsn) (dao.Repository, error)` factory versus this track's three interfaces is reconciled inside S1 rather than left as a silent mismatch. |

**Sequencing:** S3 and S2 run first and in parallel (both feed S1's shape). S1 drafts once both rule. S4 and S5 run in parallel with S1 — neither depends on it, and both unblock 04, which is last in the dependency order and should not wait on my longest story. S6 gates S1 closing.

**Order-of-service note for team-lead:** if anything in this track has to be delivered before everything else is ready, it is S1. S4 is second, because 04 has three inputs and is idle until all three land.

## Verification

- Every story's artifact exists at the path named above; `decisions/` holds one file per pattern run, each with the ⟨02⟩ model-and-turns footer.
- S1 read by a reader who has not seen this track's other docs produces no "what does this mean" questions (that is S6, run for real, not asserted).
- `find 05-data-ops -name '*.go' -o -name '*.sql' -o -name 'go.mod'` returns nothing at every point in this phase.
- `../CASTING.md` §8 has four new rows; `grep -c "profile-gen:start" 05-data-ops/CLAUDE.md` still returns 1 (hires never get markers).
- No file outside `05-data-ops/` is modified except my own row in `../PLANNING.md` and my four hire rows in `../CASTING.md` §8.
- Variance re-check at hiring time against `../CASTING.md` §8 as it then stands — rosters are approved in arrival order and mine arrives last, so I confirm the four archetypes are still at 1× before creating anyone, and revise rather than break rule 1 if another lead has taken one.
