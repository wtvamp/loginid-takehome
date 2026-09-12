# Backlog — 01 Product & Industry Research (incl. Design)

**Track:** `01-product-industry-research-design/` **Lead:** Naomi Voss **Epic:** LT-1

All seven items below are **already done** — this track closed all six `PLAN.md` stories plus its housekeeping task before Phase 4 opened. Per team-lead's instruction, a story whose acceptance criteria are already met is created and closed as done; that is the honest backlog for a design/prose track with no implementation step of its own; every "Model/effort" below reflects what actually produced the work, not a future estimate.

---

## Story 1 — Audit every claim in the framing/rationale docs for evidence (LT-6)

**Description:** Audit every quantitative or evaluative claim in `industry-framing.md`, `personas-use-cases.md`, and `api-connector-design-rationale.md` against this track's two research passes (`research-loginid-company.md`, `research-oauth-history.md`), then fix or soften every unsupported claim in place. Precondition for Story 2 — the structured written debate needs to argue over claims that have already been checked, not claims that might not survive scrutiny. Produced by Desmond Okafor (Spreadsheet archetype, claims-auditor seat), synthesized by the lead.

**Acceptance criteria:**
- [x] The receiving reader (evaluator, or another track citing this doc) can act on every remaining claim without re-deriving whether it's sourced.
- [x] `decisions/claims-audit.md` exists with a claim / source / status / resolution row for every quantitative or evaluative claim in the three docs.
- [x] Every claim marked asserted/unsourced in the audit is either softened in place or has a named evidence file — no claim ships at a confidence level the audit didn't support. Enforcement site: the edits themselves in `industry-framing.md` and `personas-use-cases.md`, cross-checked against the audit table.

**Depends on:** — (first-wave, no upstream dependency)
**Model/effort:** sonnet, medium (Spreadsheet-archetype hire; synthesis by lead)
**Type:** Task
**Labels:** `track-01`, `no-code-yet`

---

## Story 2 — Resolve whether the password baseline is a deliberate framing choice or an over-read (LT-7)

**Description:** Run a structured written debate — Position (Imogen Hale, Storyteller) argues the password/legacy-connector baseline is worth narrating as a deliberate "before/after" simplification; Rebuttal (Tobias Lindqvist, Designated Skeptic) argues that's an unfalsifiable claim about the assignment author's intent; Desmond Okafor fact-checks both; the lead synthesizes. This is the one genuinely contestable framing decision in the track, and it's binding for `industry-framing.md` §2/§4 and for the one-paragraph thesis the final submission packet uses.

**Acceptance criteria:**
- [x] Another track or an evaluator can read `industry-framing.md` §2/§4 and act on the framing without re-opening the debate itself.
- [x] `decisions/password-baseline-debate.md` preserves the full Position, Rebuttal, fact-check, and synthesis — both positions kept in the record, not only the winning one.
- [x] Every claim about what the assignment "is testing for" or what LoginID "intended" is removed or explicitly hedged as a stated belief, not an assertion — the synthesis's ruling, applied directly to `industry-framing.md`. Enforcement site: the revised document text itself (no residual first-person-certainty phrasing about evaluator/author intent).

**Depends on:** Story 1 (must run over already-checked claims)
**Model/effort:** sonnet, medium (Storyteller + Designated Skeptic + Spreadsheet hires; synthesis by lead)
**Type:** Story
**Labels:** `track-01`, `no-code-yet`

---

## Story 3 — Harden the connector/API rationale as the single source of truth for 03 (LT-8)

**Description:** Add a closing "Contract summary for 03" section to `api-connector-design-rationale.md`: exact assignment field names for both entities, the `method` discriminator and 1-to-many profile→credential requirement, the resolved endpoint list (`GET /profiles/{id}`, `POST /profiles/search` — resolved over `GET` with query params, with the reasoning stated), the provider-agnostic connector interface shape, and an explicit "out of scope for 03" list. Then cold-read the result with a genuinely context-free reader (Wesley Okonkwo, on loan from track 04) reading exactly as 03 would, and fix everything he couldn't act on without asking a question first. This is this track's highest-stakes hand-off — the file `03-engineering-delivery` implements against directly.

**Acceptance criteria:**
- [x] 03 can implement handler shapes and the connector interface from this document alone, without re-deriving the reasoning or messaging 01 a clarifying question — the hand-off standard.
- [x] `api-connector-design-rationale.md` ends with a "Contract summary for 03" section naming exact field names, the resolved search endpoint shape (including the `expand` field's exact JSON position — top-level `bool`, not nested), and an explicit out-of-scope-for-03 list.
- [x] `decisions/rationale-cold-read.md` records the cold read and every fix it produced. Enforcement site: the concrete JSON example for `expand` now present in the document, closing the one ambiguity the cold read found.

**Depends on:** Story 2 (the connector's "why this pattern" framing cites the post-debate wording of `industry-framing.md` §2)
**Model/effort:** sonnet, medium (lead-authored; cold-read reader on loan from track 04, cost borne outside this track's own budget)
**Type:** Story
**Labels:** `track-01`, `no-code-yet`

---

## Story 4 — State product requirements for search authorization (LT-9)

**Description:** Add a "Search authorization — product requirements" appendix to `personas-use-cases.md`: what Persona 2 (support/ops analyst) legitimately needs from the search API (name/phone partial-match, masked-by-default results, a deliberate auditable "expand" action, pagination, per-caller attribution, an independent volume limit distinct from attribution, visibility independent of query specificity) and what Persona 2 must never be able to do (bulk export, unconstrained queries, unmasked PII in the search response itself, any unlogged path). Product requirements only — no scope names, token formats, or policy mechanics; this is input to `02-ai-security-architecture`'s authorization-scoping decision, not a substitute for it.

**Acceptance criteria:**
- [x] 02 can read this appendix and design `profile:search` vs. `profile:read:own` scoping without 01 having pre-decided any policy mechanic for them.
- [x] Both the "legitimately needs" and "must not do" lists are present in `personas-use-cases.md`, including the two items added by the rotating devil's-advocate objection (volume-limiting independent of attribution; visibility not solely query-specificity-dependent).
- [x] No scope name, token format, or rate-limit number appears in the appendix — checked against the track's own "does not own" boundary in `./CLAUDE.md`. Enforcement site: the appendix text itself, and 02's `api-auth-design.md` as the consuming document.

**Depends on:** — (independent of Stories 1–3; consumed by 02)
**Model/effort:** sonnet, medium (lead-authored; Designated Skeptic rotating-advocate objection seat)
**Type:** Story
**Labels:** `track-01`, `no-code-yet`

---

## Story 5 — Write UX notes for the two surfaces on top of this backend (LT-10)

**Description:** One-page, text-only UX notes for the admin profile-search console (Persona 2) and the onboarding pre-fill flow (Persona 3) — screens and the one design rule each surface enforces (masked-by-default + expand-is-a-distinct-action; the partner password never visible past token exchange). No wireframes, no design canvas — this is a backend-focused assignment, and the notes exist to support the narrative, not compete with it. A rotating devil's-advocate pass ("which of these sentences is a claim?") checked the draft for unfalsifiable or unsourced assertions dressed as screen descriptions.

**Acceptance criteria:**
- [x] A reader can state each surface's one enforced design rule and see how every listed screen decision follows from it, without needing a wireframe.
- [x] `ux-notes.md` exists, is one page, and contains no wireframes or design-canvas artifacts.
- [x] Every sentence in the document is either a screen/behavior description or an explicit restatement of an already-established backend requirement — no unfalsifiable claim about user comprehension or outcome. Enforcement site: the two sentences the rotating-advocate pass flagged and rewrote as intent statements ("...consistent with the backend treating expanded access as a distinct, auditable event"; "...to signal whose credential this is").

**Depends on:** Story 4 (references the same masked-by-default/expand distinction)
**Model/effort:** sonnet, medium (lead-authored; Spreadsheet rotating-advocate objection seat)
**Type:** Task
**Labels:** `track-01`, `no-code-yet`

---

## Story 6 — Evaluator-legibility cold read of the industry framing thesis (LT-11)

**Description:** A second, lighter cold read (same loaned reader, second of two turns) of `industry-framing.md` after its Story 2 revision: can a reader with no other project context state the thesis and the "industry-standard baseline, not a claim about the assignment author's intent" distinction correctly, in their own words, after one pass? This tests evaluator legibility specifically, since `industry-framing.md` (unlike the Story 3 hand-off) is read by evaluators, not acted on by another track.

**Acceptance criteria:**
- [x] A cold reader can restate the document's thesis and the before/after-vs-author-intent distinction correctly without needing to open `decisions/password-baseline-debate.md` directly.
- [x] `decisions/rationale-cold-read.md` records the reader's restatement and confirms no fix was needed — a legibility test that passes clean is itself the acceptance evidence, not a null result to hide. Enforcement site: the reader's restatement, recorded verbatim, checked against the document's own stated thesis.

**Depends on:** Story 3 (shares the same loaned reader and loan-turn budget)
**Model/effort:** sonnet, medium (borrowed reader; no lead-side authoring needed since the read came back clean)
**Type:** Task
**Labels:** `track-01`, `no-code-yet`

---

## Task 1 — Track housekeeping (hiring, registry, status) (LT-12)

**Description:** Non-deliverable housekeeping required for the track to run as part of the org: spawn the three hires as standing teammates, append their `CASTING.md` §8 registry rows, keep the `PLANNING.md` row current at each milestone, and keep `./CLAUDE.md`'s Status and AI-tooling note current at turn boundaries.

**Acceptance criteria:**
- [x] All three hires (`imogen-hale`, `desmond-okafor`, `tobias-lindqvist`) exist as persona files, `.claude/agents/` definitions, and `CASTING.md` §8 rows, and were spawned as standing teammates in this track's top-level session.
- [x] `PLANNING.md` row 01 reflects the track's actual current state at every milestone (plan drafted → hires spawned → S1–S6 done).
- [x] `./CLAUDE.md` Status and AI-tooling note name every hire, every pattern run, the models used, and the one research pass authorized (OIDC `address`-claim verification, `research-oidc-address-claim.md`). Enforcement site: `grep -c "profile-gen:start" */CLAUDE.md CLAUDE.md` from repo root still totals 6 (hires are never added as markers).

**Depends on:** — (runs alongside all other stories)
**Model/effort:** sonnet, medium (lead-authored housekeeping)
**Type:** Task
**Labels:** `track-01`, `no-code-yet`

---

## Story 7 — Address cross-track consistency pass findings _(placeholder)_ (LT-13)

**Description:** Reserved for whatever the Phase 3 cross-track consistency pass (run by team-lead, fable/high, once 02/03/05 close) sends back to this track — a contradiction between this track's docs and another track's, a claim that doesn't hold up against another track's finished decision, or a hand-off gap the pass finds. Empty until the pass reports; created now so the Epic's story list doesn't need a second Jira-authoring pass later.

**Acceptance criteria:**
- [ ] Every finding the consistency pass assigns to 01 is either fixed in the relevant document or explicitly recorded as a decision not to change, with reasoning, in a `decisions/` file.
- [ ] No finding is silently dropped — the pass's report and this track's response to each item are both traceable in `LOG.md` or a `decisions/` file.

**Depends on:** Cross-track consistency pass (root, Phase 3) — Dana Whitfield
**Model/effort:** sonnet, medium (same hire roster, same rotating-advocate/claims-audit patterns as needed)
**Type:** Story
**Labels:** `track-01`, `from-consistency-pass`, `no-code-yet`
**Status:** Open — not yet actionable, no findings received.
