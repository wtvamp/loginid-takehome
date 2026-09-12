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

**Complete — eight stories (six planning, two from the cross-track consistency pass); `../PLANNING.md` row reads "plan approved". Jira Epic `LT-5`, stories `LT-14`…`LT-20`.** Nothing downstream is blocked on this track.

The consistency pass (`../decisions/cross-track-consistency.md`) and its re-run named this track owner on ten findings across three rounds — all closed. Two were errors in this track's own documents that four reviewers, three cold reads and a plan approval had all missed — **both invisible to any single-document review because each contradiction lived between two files**: `migration-approach.md` was marked *final* while carrying a provisional layout its own closed dependency had already overruled (F28), and the retention mechanism specified in `pii-governance.md` had no method on the DAO interface to be implemented against, so a policy this track ruled on was unreachable and no track owned the gap (F21). Both fixed; the second added `DeleteExpired` and `DeleteProfile` to the contract as an addendum. That is the concrete evidence behind the first of two method findings this track contributed to `../LOG.md`: *a review scoped to a document can only find errors that are in a document; relationships between artifacts need either a reviewer scoped to the graph, or a representation that makes the graph first-class.*

The second came out of the re-run and is the more useful because it is a **check rather than a review**: *a summary of another document is a dependency on it, and nothing in a document-scoped review structure treats it as one — but restatements are greppable.* This track produced five instances of the same failure (a stale cross-reference in `PLAN.md`, a provisional layout in `migration-approach.md` its own closed dependency had overruled, a §3c summary describing pre-review signatures, a `backlog.md` story description, and a Jira issue transcribed before the fix — the last having left the repository entirely). It is now **seam 13** of the org's consistency checklist: every restated signature, table shape or config value grep-verified against its authoritative source. Running it against this track found three further instances, one of which a human-scoped pass over the same file had missed, because a reviewer reasonably skips a superseded preamble and a grep does not know the difference.

The governing distinction, adopted org-wide: **strike through a superseded claim; delete a superseded copy, with a banner recording what was removed and why.** A claim is evidence of what someone once believed. A copy of another document's contents is a liability whose only value was being accurate — which is why the stale interface listing at the head of `multi-db-strategy.md` was removed rather than corrected.

- **The Go-shaped contract** is in `multi-db-strategy.md` and has been accepted by 03 after their own independent review. It carries interface signatures, a per-method summary table, field-level schema with nullability, error semantics and the sentinel set, and a twelve-item "out of scope for 03" list in which **every item names its enforcement site** — a DDL constraint, an index, a normalization step, or a conformance test. Two items name DAO-layer validation as their site and are flagged as the contract's weakest links, because the discipline is worthless if it only records the cases where a clean site existed.
- **`migration-approach.md`** is final and delivered to 04.
- **`pii-governance.md`** carries proposed retention windows and the deletion mechanism, with every basis cell either an engineering rationale or the honest word *unknown* — storage limitation is a principle demanding a justified window, not a number anyone can look up, and a citation implying otherwise would be a false statement in a client-facing document.
- Three claims in this track's earlier prose were found to be **wrong and are corrected in place with the correction visible**, rather than silently edited.

Cross-track contradictions settled directly with their owners: pagination (05 keeps `Offset`; the API boundary exposes an opaque cursor over it, since a keyset scheme resists enumeration no better and the real control is 02's cumulative counter) and phone search (exact after E.164 normalization, with the partial-match need real for names and not phones — 01 accepted and recorded it with this track's index as the enforcement site).

Two things are deliberately left open rather than resolved: whether goose's default version table carries a unique `version_id` (unverified; the recommendation does not depend on it), and whether subject-deletion should reach derived data such as logs and connector traces (unmodelled, no clock and no key — recorded so its absence is not mistaken for a ruling, with the upstream control named: if PII never enters a log, there is nothing to delete from it).

No implementation code exists in this track and none should until Warren's explicit go-ahead.

<!-- profile-gen:start slug=priya-nandakumar -->
@profiles/priya-nandakumar/priya-nandakumar.md
<!-- profile-gen:end slug=priya-nandakumar -->
