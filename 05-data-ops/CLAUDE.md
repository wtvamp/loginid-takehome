# Track: Data Ops

Refer back to `../CLAUDE.md` for the full assignment text and how this track fits with the other four.

## Mission

Own the data model and the multi-database strategy that question 1 requires, plus the data governance implications of storing `user_profile` PII and `user_credential` secrets. The engineering track implements this track's design; this track does not write the Go DAO code itself.

## Owns

- Schema design for `user_profile` (name, address, phone) and `user_credential` (username, method, password), including how `method` is modeled (e.g., an enum for password vs. other auth methods) and how the two entities relate.
- The multi-database abstraction strategy: how one DAO interface serves PostgreSQL/CockroachDB and SQLite without diverging behavior — e.g., a repository interface with per-driver implementations, and which SQL dialect differences (upserts, autoincrement, JSON columns) need to be handled.
- Migration strategy: how schema changes would be versioned and applied across the supported databases.
- Data governance for PII: retention policy for `user_profile` data and for any cached data pulled from the third-party IDP connector's `/identity` endpoint, and how that interacts with credential storage (`user_credential` should never store PII, and the reverse).
- Indexing/query design to support "search and retrieve" from question 2 (e.g., what fields the API needs to search on, and what that implies for schema indexes).

## Does not own

- Access control and transport security for that data — that's `../02-ai-security-architecture/CLAUDE.md`.
- Writing the DAO code itself — that's `../03-engineering-delivery/CLAUDE.md`; this track hands it a schema and an interface contract to implement.
- Where the databases actually run and how migrations are executed in a pipeline — that's `../04-infra-devops/CLAUDE.md`.

## Deliverables

- Schema definitions (DDL or equivalent) for `user_profile` and `user_credential`, across the target databases.
- A written multi-database abstraction strategy the engineering track can implement directly.
- A migration approach.
- A short PII retention/governance note.

## AI tooling note

Built with **Claude Code**. The track lead (this persona) ran as its own top-level session on **Claude Opus 5** — chosen deliberately, not by default: this track's planning is low-volume but carries asymmetric risk, since a wrong nullability or a wrong interface signature propagates into tracks 03 and 04 and is expensive to reverse. Four **Claude Sonnet 5** hires sat in named debate seats (`../CASTING.md` §8). One Sonnet subagent ran the lead-authorized research pass; per `../CASTING.md` §3, deep research is authorized and run only by leads, and hires request it rather than performing it.

Four orchestration patterns from `../CASTING.md` §5 produced this track's decisions, each ending in a durable artifact under `decisions/` rather than in chat:

| Pattern | Decision | Artifact |
|---|---|---|
| Three hats | the multi-DB abstraction | `decisions/multi-db-abstraction.md` |
| Adversarial pair | the `Search()` query shape | `decisions/search-query-shape.md` |
| Newcomer's question | legibility of the 05→03 hand-off | `decisions/handoff-legibility-05-03.md` |
| Rotating devil's advocate | the migration approach | `decisions/migration-approach-objections.md` |

**What the multi-agent structure actually bought, stated plainly because it is the part worth grading.** On three separate occasions a hire left the lead's *conclusion* standing and destroyed the *argument* underneath it: the per-driver ruling (rejected on a "scattering" of dialect branches that a count showed to be two conditionals — so the real basis is reviewability, not cost), the error-translation rule (self-contradicting, and unimplementable on SQLite, which has no SQLSTATE at all), and R2 of the migration approach (a premise corrected in research while the conclusion was left resting on the superseded version). The conclusions were mostly right; the stated reasons were mostly the first plausible one to hand. A single-agent version of this track would have produced the same answers with unfalsifiable reasoning behind them.

One methodological choice worth recording: the cold-read seat was kept **deliberately unfamiliar** — that hire was instructed not to read this track's reasoning documents and sat out two debates to stay a usable instrument. She returned 14 blocking items against a contract three other reviewers had already passed. She could not say afterwards which of the 14 she would still have found had she read the debates first, and that uncertainty is the argument for paying the cost again.

## Status

**Complete — all six stories closed; `../PLANNING.md` row reads "plan approved".** Nothing downstream is blocked on this track.

- **The Go-shaped contract** is in `multi-db-strategy.md` and has been accepted by 03 after their own independent review. It carries interface signatures, a per-method summary table, field-level schema with nullability, error semantics and the sentinel set, and a twelve-item "out of scope for 03" list in which **every item names its enforcement site** — a DDL constraint, an index, a normalization step, or a conformance test. Two items name DAO-layer validation as their site and are flagged as the contract's weakest links, because the discipline is worthless if it only records the cases where a clean site existed.
- **`migration-approach.md`** is final and delivered to 04.
- **`pii-governance.md`** carries proposed retention windows and the deletion mechanism, with every basis cell either an engineering rationale or the honest word *unknown* — storage limitation is a principle demanding a justified window, not a number anyone can look up, and a citation implying otherwise would be a false statement in a client-facing document.
- Three claims in this track's earlier prose were found to be **wrong and are corrected in place with the correction visible**, rather than silently edited.

Two things are deliberately left open rather than resolved: whether goose's default version table carries a unique `version_id` (unverified; the recommendation does not depend on it), and whether subject-deletion should reach derived data such as logs and connector traces (unmodelled, no clock and no key — recorded so its absence is not mistaken for a ruling, with the upstream control named: if PII never enters a log, there is nothing to delete from it).

No implementation code exists in this track and none should until Warren's explicit go-ahead.

<!-- profile-gen:start slug=priya-nandakumar -->
@profiles/priya-nandakumar/priya-nandakumar.md
<!-- profile-gen:end slug=priya-nandakumar -->
