# Backlog — Track 05, Data Ops

**Track:** 05-data-ops · **Lead:** Priya Nandakumar · **Epic title:** *Data model, multi-database DAO contract, and PII governance* · **Epic key:** LT-5

Authored by the track lead per `../PLAN.md` Phase 4. Team-lead is the single Jira writer; this file is the source and is itself a submission artifact. Stories are in dependency order.

All seven stories carry `no-code-yet`: no `.go`, `.sql` or `go.mod` exists in this track and none should until Warren's explicit code go-ahead. Stories 1–6 are **delivered** — this track's Phase 3 work is complete — and are transcribed as the record of what was decided and where it lives. Story 7 is open and awaiting the consistency pass.

**One deviation from the stated format, flagged rather than fudged.** The format asks for *Depends on* entries by other tracks' story titles. This track's stories have almost no upstream dependencies — 05 is first-wave and on the critical path — so nearly every cross-track link runs the other way. I have therefore added a **Blocks** line where the relationship is downstream, and I name the counterpart *descriptively* rather than inventing a title from 03's or 04's backlog that I have not read. Team-lead has both sides and can link them accurately; guessing another track's story titles would have produced links that look right and point nowhere.

---

## Story 1 — Rule the multi-database abstraction shape (LT-14)

**Type:** Story · **Labels:** `track-05`, `no-code-yet` · **Model/effort:** opus/high (lead synthesis) + 3× sonnet (debate seats) · **Status:** delivered

**Description.** Decide how one DAO interface serves PostgreSQL, CockroachDB and SQLite: per-driver implementations behind one interface, a single implementation with dialect branches, or codegen (sqlc). The existing recommendation in `multi-db-strategy.md` had been reached by one agent reading sources and never attacked, and codegen had been name-checked without ever being argued. Run as a three-hats debate; ruling and all three positions in `decisions/multi-db-abstraction.md`. Outcome: per-driver confirmed — **but the document's stated reason was wrong and is corrected in place.** Of the six concerns in its own dialect table, exactly two survive as real conditionals, so the two options cost roughly the same and the choice is reviewability, not cost. Codegen proved orthogonal to the question rather than a competing option: sqlc generates per-engine Queriers satisfying no common interface, so it fills the boxes per-driver implementations draw. It is left to 03 as an implementation preference.

**Acceptance criteria.**
- [x] The receiving track can act on this without re-deriving the reasoning.
- [x] The ruling names which option was chosen, on what grounds, and states plainly that the grounds are reviewability rather than cost.
- [x] All three positions preserved verbatim, not summarized into agreement.
- [x] Every correction to the prior document is visible as a correction, not a silent edit.
- [x] Pattern footer records model per seat and hire turns consumed.
- [x] Enforcement site for the ruling named: package boundaries, plus the conformance suite in Story 3 without which the ruling does not hold.

**Depends on:** none. **Blocks:** Story 2 and Story 3 (this track).

---

## Story 2 — Settle the `Search()` query shape (LT-15)

**Type:** Story · **Labels:** `track-05`, `no-code-yet` · **Model/effort:** opus/high (lead ruling) + 2× sonnet (Proposer, Skeptic) · **Status:** delivered

**Description.** `Search()` is the flagship query of the DAO contract and the only genuinely open design question in it — every other method is fixed by the assignment text. Settle the `ProfileQuery` field set, per-field match semantics, filter combination, pagination form, sort order and stability, what `total` counts, and how the documented SQLite parity gap surfaces to a caller. Run as an adversarial pair; ruling and all objections in `decisions/search-query-shape.md`. Eight numbered objections raised, eight accepted, none declined. Four were live defects: `ORDER BY name` returning different page-1 rows per backend (PostgreSQL collation vs. SQLite `BINARY` byte order); `LOWER(name) LIKE` unable to use a GIN index declared on the bare column, so the accelerator accelerated nothing; caller input reaching a `LIKE` pattern unescaped; and `Country` normalized into the query but not into the table.

**Acceptance criteria.**
- [x] The receiving track can act on this without re-deriving the reasoning.
- [x] Every objection closes with an explicit "what would change my mind"; every one answered in writing, with declines stated rather than silent.
- [x] Each caller-visible guarantee names its enforcement site — collation clause in DDL plus `ORDER BY` expression plus conformance test; expression index with the textual-match rule; DAO-side escaping plus `ESCAPE '\'`; `CHECK` constraint plus write-side normalization.
- [x] A guarantee deliberately *not* made is also recorded with its site as "none — accepted" (no index supports the ordering; every `Search` sorts its filtered set).
- [x] Pattern footer records model per seat and hire turns consumed.

**Depends on:** Story 1. **Blocks:** Story 3.

---

## Story 3 — Deliver the Go-shaped DAO contract to track 03 (LT-16)

**Type:** Story · **Labels:** `track-05`, `security-graded`, `no-code-yet` · **Model/effort:** opus/high — asymmetric risk: a wrong nullability or interface signature propagates into 03 and 04 and is expensive to reverse (`../02-ai-security-architecture/planning-approach.md` §1) · **Status:** delivered and accepted by 03

**Description.** Turn the prose strategy in `multi-db-strategy.md` into a contract 03 can implement without re-deriving this track's reasoning: interface signatures, a per-method summary table, domain types, the full sentinel set with translation rules, field-level schema with nullability per backend family, indexes, and a numbered out-of-scope list. `security-graded` because it specifies credential storage — `secret`, `secret_state`, `hash_algo`, `hash_cost` — which falls under 02's independent second-review rule. Includes the factory-shape resolution agreed directly with 03 (`dao.Repository` as a composite exposing `Profiles()`/`Credentials()`/`Methods()`, keeping `dao.New(driver, dsn)` unchanged; kept an interface so 03 can mock it) and the write-path retry seam required by `research-cockroachdb-postgres-semantics.md`. Reviewed by 03's own Detail Hawk after delivery; four findings accepted, one of which added `CreateProfileWithCredential` — the contract had barred 03 from composing a cross-call transaction while supplying no atomic path for registration, leaving a window where a profile exists with PII and no credential.

**Acceptance criteria.**
- [x] 03 can implement without re-deriving the reasoning — **verified by 03 reading it cold and confirming, not asserted by the author.**
- [x] Every caller-visible guarantee names its enforcement site; the two weakest sites (`is_active`, and `requires_secret` ↔ `secret_state`, both unable to be DDL constraints because `CHECK` cannot reference another table) are named as weakest rather than quietly listed.
- [x] Nullability stated per column, per backend family, with "absent" single-valued: NULL means absent, `''` is invalid, enforced by named `CHECK` constraints.
- [x] Error sentinels enumerated with trigger conditions; translation by structured error codes plus constraint names defined in our own DDL, never message text.
- [x] Out-of-scope list numbered, each item naming why and whom to flag.
- [x] Counter-intuitive behaviours warned *before* a reader meets them.
- [x] Contract carries no `.go` or `.sql` file — Go-shaped prose only.

**Depends on:** Stories 1 and 2 (LT-14, LT-15). **Blocks:** Story 6 / LT-19 — the migration series creates this contract's tables, so migrations depend on the contract and not the reverse; 03's DAO implementation story (title TBD — 03's backlog).

> *Corrected after transcription.* This line originally read "Stories 1, 2, 6", which made the migration approach block the contract. It was wrong in both membership and direction, and it is a stale reference carried out of `../PLAN.md` story S1 — where "S4" named the migration approach under the pre-backlog numbering. The contract never consumed it. Noted rather than silently fixed because the error survived a plan approval, four debates and three cold reads before a literal transcription into Jira exposed it: every reviewer was reading the documents, and nobody's review was scoped to read the dependency graph.

---

## Story 4 — Cold-read the 05→03 hand-off and fix what fails (LT-17)

**Type:** Story · **Labels:** `track-05`, `no-code-yet` · **Model/effort:** sonnet (cold reader) + opus/high (revisions) · **Status:** delivered

**Description.** Test the contract against the hand-off standard this track asked the project to adopt, using a reader deliberately kept unfamiliar with the track's reasoning. The hire was instructed at spawn not to read `multi-db-strategy.md`, `pii-governance.md` or the research docs, and sat out two debates to remain a usable instrument; she declared her one contamination (`PLAN.md` told her what the contract *intended* to contain) unprompted and reported against text rather than intent. Result in `decisions/handoff-legibility-05-03.md`: **14 blocking items against a contract three other reviewers had already passed; all 14 accepted and fixed,** plus three tail findings adopted. A parallel cold read of `migration-approach.md` by a hire on loan from 04 ran alongside.

**Acceptance criteria.**
- [x] Reader is genuinely cold; the restriction and its cost are recorded, and any contamination declared.
- [x] Every finding either fixed or recorded with a reason for declining; none declined here.
- [x] Surprise-versus-warning audit performed for each counter-intuitive behaviour, with placement corrected where a reader met the surprise first.
- [x] Jargon undefined on first use catalogued and defined.
- [x] Cross-track consequences of legibility gaps traced — a `pg_trgm` glossary gap had already propagated into a missing migration-runner credential row in 04's secrets inventory.

**Depends on:** Story 3 (drafted). **Blocks:** Story 3 (closing) — deliberately circular: the cold read gates the contract being called stable.

---

## Story 5 — Set PII retention windows and the deletion mechanism (LT-18)

**Type:** Story · **Labels:** `track-05`, `security-graded`, `no-code-yet` · **Model/effort:** sonnet (author) + opus/high (ruling) · **Status:** delivered

**Description.** `pii-governance.md` previously stated the policy shape and stopped at "a product/legal decision this track doesn't own outright" — true, and not actionable: 04 cannot build a sweep against a principle. This story puts proposed numbers in a table and specifies the mechanism they plug into. Windows: 24 months for `source='direct'`, 30 days for `idp_cache`, 7 days for orphaned cache rows, and no independent window for `user_credential` (wholly governed by its profile via the cascade FK). **Every basis cell is either an engineering rationale or the honest word *unknown* — there is no third kind**, because storage limitation under GDPR Art. 5(1)(e) is a principle demanding a justified window, not a number anyone can look up, and a citation implying otherwise would be a false statement in a client-facing document. Adds a `deletion_log` table to the schema, PII-free by construction, `profile_id` deliberately carrying no FK because the row it names is meant not to exist.

**Acceptance criteria.**
- [x] 04 can build the sweep without re-deriving the reasoning.
- [x] Every proposed number marked as a POC default pending a product/legal ruling; unknown bases and ratifiers left visibly empty rather than filled with plausible citations.
- [x] Orphan rows keyed on `created_at`, not `updated_at` — a clock the read path touches lets a row reset its own expiry and never die.
- [x] The cache window documented as "since last use", not "since first seen", with its relationship to the orphan rule stated as one rule seen from two sides.
- [x] Both cascade consequences stated loudly: deleting a profile destroys its credentials; **deleting a credential does not delete the profile**, so a user removing their last login method still has PII on the `direct` clock. Named as a product decision that is currently *not* what the schema does.
- [x] Deletion is hard, not soft-flagged; subject deletion reuses the sweep's code path; `deletion_log` holds no PII, with a conformance test asserting it.
- [x] Health check specified as a requirement: rows examined, rows deleted, and **age of the oldest surviving row per class** — the third being the only signal that detects a silently stopped sweep, since rows-deleted cannot distinguish a broken sweep from an empty one.

**Depends on:** none. **Blocks:** Story 6; 04's retention-sweep and observability work (title TBD — 04's backlog; already folded into their `observability.md`).

---

## Story 6 — Deliver the migration approach to track 04 (LT-19)

**Type:** Story · **Labels:** `track-05`, `no-code-yet` · **Model/effort:** opus/high + sonnet (objection seat) · **Status:** delivered, final

**Description.** One of the three inputs 04 was gated on. `migration-approach.md` specifies the tool (goose), the three-directory layout with its two-invocation mechanism and separate version tables, the seed-data rule, and five requirements on 04 — migrations as a pre-start step, single-runner exclusion, fail-closed when the schema is behind, two distinct database credentials, and rollback not being a data-recovery plan. Put through a devil's-advocate objection turn (`decisions/migration-approach-objections.md`): six objections, six accepted. **The most valuable overturned the reasoning behind the document's central requirement while leaving the requirement standing** — R2 had argued the database cannot provide a lock because CockroachDB lacks advisory locks, a premise already corrected in this track's own research. The true reason is scope: a transaction-scoped lock releases between goose files and cannot cover a multi-file run, while session-scoped locks that would cover it are the ones CockroachDB does not support.

**Acceptance criteria.**
- [x] 04 can act without re-deriving the reasoning — verified by a cold read from a 04 hire on loan, all findings fixed.
- [x] Every requirement states whether it is a requirement (mechanism 04's choice) or a mechanism (and why the requirement cannot be met otherwise).
- [x] Fail-closed checks **both** version tables, not one — checking only `shared/` would pass with the trigram index unapplied.
- [x] Migration-runner and runtime credentials separated, with `CREATE EXTENSION` privileges named as PostgreSQL-only.
- [x] Unverified facts left visibly open rather than asserted — whether goose's default version table carries a unique `version_id`, with a recommendation that does not depend on the answer.
- [x] Tool-independence claim names its one exception rather than overclaiming.

**Depends on:** Story 5 (`deletion_log` belongs in the shared migration series). **Blocks:** 04's migration-job and CI stages (title TBD — 04's backlog; "provisional" now dropped on their side).

---

## Story 8 — Add the retention/deletion methods to the DAO contract (F21) (LT-50)

**Type:** Story · **Labels:** `track-05`, `security-graded`, `no-code-yet`, `from-consistency-pass` · **Model/effort:** opus/high (contract addendum) + sonnet (03's first-pass review) · **Status:** contract change delivered; awaiting 03's addendum review · **Jira key:** LT-50

**Description.** The cross-track consistency pass (F21) found that `pii-governance.md` specified a bulk retention sweep and a subject-deletion request sharing one code path, each writing a `deletion_log` row, and `multi-db-strategy.md` §5 defined the table — but **no method on the interface could reach any of it.** `Profiles().Delete` wrote no log row, and §3 forbids 03 from opening a transaction to combine the two, so a policy this track ruled on could not be implemented against the contract this track wrote, and no track owned the gap. Fixed by adding two composite-level methods in `multi-db-strategy.md` §3c: `DeleteExpired(ctx, class, olderThan, maxRows) (SweepResult, error)` and `DeleteProfile(ctx, id, externalRef)`, both writing `deletion_log` in the same transaction as the delete (signatures as finalized after 03's review — batched, and with `reason` asserted by the method rather than passed by the caller). `SweepResult` returns rows examined, rows deleted, and the age of the oldest surviving row — the last being the only signal that detects a silently stopped sweep, since rows-deleted cannot distinguish a broken sweep from an empty one.

**Acceptance criteria.**
- [x] Both methods sit on the composite, not on a sub-repository — they span `user_profile` and `deletion_log`, and the DAO is the only layer permitted to open a transaction.
- [x] One retention class per call; never a mixed-class predicate.
- [x] Each class uses its own clock column — `created_at` for orphans, because a clock the read path touches lets a row reset its own expiry.
- [x] `OldestSurvivingAt` returned from the DAO rather than left for 04 to compute, since only the DAO can answer it in the same query plan.
- [x] `Profiles().Delete` deliberately still writes no log row, with the reason recorded.
- [x] The complete set of cross-table operations is stated as three; a fourth requires a contract change.
- [x] **03 accepted the addendum** after a Detail Hawk first-pass review that raised four findings, all accepted: `OldestSurvivingAt`'s nil case narrowed to "class is empty" only (three conditions had been collapsing into it, including the two the metric exists to distinguish); `RetentionClass` zero value given an explicit conformance assertion; **`reason` removed as a parameter** on `DeleteProfile` and asserted by the method, since a caller-supplied reason code makes an audit trail's reasons caller-assertable; and `DeleteExpired` bounded by a required `maxRows` with one transaction per batch, never one per sweep, plus a never-request-scoped-context requirement.
- [x] **An interaction between two of those fixes, raised by neither, caught at ruling:** batching makes deletable rows remain mid-sweep by construction, so `OldestSurvivingAt` would be old on every batch but the last and would fire the alert every run. It is therefore meaningful **only when `Drained` is true**. Two individually correct fixes would have shipped a flapping alert; a health signal that cries wolf gets muted, at which point it is not a health signal. Relayed to 04 so the alert rule is drained-gated.

**Depends on:** Story 3 (LT-16), Story 5 (LT-18). **Blocks:** 03's **S11** — the sweep-invocation and subject-deletion-request code that calls `DeleteExpired`/`DeleteProfile` and surfaces `OldestSurvivingAt` to 04's observability (Jira key TBD — 03's backlog).

> *Ownership of the sweep job, resolved.* The pass found that no track owned writing it: the contract made it writable, 03 had no such story, and `pii-governance.md` gave 04 only *where* jobs run. Renata claimed it as 03's S11 on 2026-09-12. The boundary is now three-way and clean — **05** defines the methods and what the metrics mean, **03** writes the code that calls them, **04** decides where it runs and reads the metrics. That was the orphan; it is no longer one.

---

## Story 7 — Extend subject-deletion to derived data *(owner ruled: 04)* (LT-20)

**Type:** Story · **Labels:** `track-05`, `security-graded`, `no-code-yet`, `from-consistency-pass` · **Jira key:** LT-20 · **Model/effort:** TBD with owner · **Status:** **the pass ruled the enforcement site is 04's log-sink retention policy.** This row is retained as 05's linked dependency — 05 supplies the definition of what counts as PII; it is not a 05-owned story and LT-20 may be re-parented under 04 accordingly. My earlier guess that 02 owned the site (never-log list as the real control) was not what the pass ruled; 02's never-log list remains the cheaper upstream mitigation but the retention of what is already logged is 04's.

**Description.** A subject-deletion request currently reaches `user_profile` and, by cascade, `user_credential`. **It does not reach derived data** — application logs, metrics, connector traces, or anything else that may have incidentally captured a name, phone or address in transit. That data has no `source` column, no retention clock, and in most cases no primary key to select on. This is **not** a ruling that derived data is out of scope for deletion; it is an unmodelled area, recorded so its absence is not mistaken for a decision. Raised by this track's Spreadsheet hire against her own stated blind spot — everything else became a row because it fit a column, and this resisted the table.

**Proposed resolution, offered rather than assumed:** the cheapest control is upstream of deletion entirely. If PII never enters a log, there is nothing to delete from it — which makes 02's never-log list the real control here, not a deletion job this track could specify. That argues for the enforcement site sitting with 02 (what may be logged) and 04 (log retention and where logs physically live), with 05 supplying only the definition of what counts as PII.

**Acceptance criteria (draft — to be confirmed by the owning track).**
- [ ] The enforcement site is named and owned by exactly one track, with the other two linked rather than assumed.
- [ ] The decision records whether derived-data deletion is in scope, out of scope, or mitigated upstream — an explicit ruling either way, not continued silence.
- [ ] If mitigated upstream, 02's never-log list names the PII fields from `multi-db-strategy.md` §5 explicitly rather than by category.
- [ ] Whatever is decided is reachable from `pii-governance.md`, so the governance note does not imply a completeness it lacks.

**Depends on:** Story 5. **Blocks:** nothing yet. **Linked:** 02 (never-log list), 04 (log retention).

---

*AI tooling note: authored by Claude Opus 5 in the 05 track-lead session from this track's six closed planning stories and four decision artifacts. Transcription to Jira project `LT` is team-lead's; this track does not write to Jira.*

---

## Cross-track links (resolved)

Resolved and created in Jira by the scribe once 01/02/03/04 were all transcribed. Table replaces the earlier pending list.

| 05 key | Relationship | Counterpart | Note |
|---|---|---|---|
| LT-16 (Story 3) | blocks | LT-35 (03 S2 — DAO interface, domain types, sentinels, factory) | resolves "03's DAO implementation story" |
| LT-16 (Story 3) | blocks | LT-19 (05 Story 6 — migration approach) | intra-track, not cross-track — supersedes the original "blocks 04's migration-execution work via Story 5" guess, which was itself corrected in place above (Story 3's Blocks line now reads "Story 6 / LT-19") |
| LT-18 (Story 5) | blocks | LT-49 (04 Story 5 — Observability) | resolves "04's retention-sweep and observability work" |
| LT-18 (Story 5) | blocks | LT-20 (04 Story 6 — log-sink retention for the audit trail, re-parented from 05) | 04's Story 6 depends on this track's `pii-governance.md` audit-log retention-window row |
| LT-19 (Story 6) | blocks | LT-45 (04 Story 1 — Containerize) | migration tooling name feeds the migration `Job` |
| LT-19 (Story 6) | blocks | LT-46 (04 Story 2 — Secrets delivery) | migration-runner credential provisioning, R4 |
| LT-19 (Story 6) | blocks | LT-47 (04 Story 3 — CI/CD pipeline) | resolves "04's migration-job and CI stages" |
| LT-20 (04 Story 6, ex-05 Story 7) | depends on | LT-27 (02 S7 — hand-off 02→04: secrets inventory and never-log list) | resolves "linked to 02's never-log list" as an actual Blocks link (LT-27 blocks LT-20) |

"Linked to 04's log retention" from the original pending list is no longer a cross-track link — LT-20 **is** that 04 story now (re-parented), so the relationship collapsed into identity, not a link.
