# Track 01 Plan — Product & Industry Research (incl. Design)

Lead: Naomi Voss (`naomi-voss`). Epic: **01 Product & Industry Research**. Governing files: root `CLAUDE.md`, `../PLAN.md` (org plan, Phase 1), `../PLANNING.md` (my row), `../CASTING.md` (palette, variance rules, persona standard, pattern catalog), `../02-ai-security-architecture/planning-approach.md` (coordination rules, hand-off standard, dependency order). This plan is written in plan mode per Phase 1 and is copied verbatim to `./PLAN.md` on exit; the real gate is team-lead's explicit "hiring go".

## Context

This track's three first-wave deliverables already exist and are complete as drafts: `industry-framing.md` (the "before/after" thesis — the assignment's password baseline is the deliberate "before", LoginID's passkey product is the "after", and the submission's job is to build the before honestly with the seams marked), `personas-use-cases.md` (three personas mapped to the three assignment questions), and `api-connector-design-rationale.md` (the field/endpoint/connector-shape rationale, designated by `planning-approach.md` §2 as the single source of truth 03 implements against). Two research docs (`research-loginid-company.md`, `research-oauth-history.md`) are their evidence base.

The work this plan schedules is therefore not drafting — it is **stress-testing and hardening**: the framing has never been argued against, the claims have never been audited for evidence, and the hand-off doc has never been read cold by someone who has to act on it. Those three gaps are exactly what the three hires and the debate plan address. 01 is first-wave and upstream of everyone (nothing hard-blocks on it, but 03 should read the rationale before finalizing), so this work should finish before 03 finalizes handlers and the connector interface.

Model/effort: the lead seat runs at whatever Warren has set for the session; all three hires are pinned `sonnet` (inherit effort); any research fan-out I run is `haiku` (project settings pin it to low). No code anywhere in this track — it is a prose track by definition.

## 1. Deliverables & tasks (each = one Jira Story under the 01 Epic)

Every row names the hand-off file it produces or consumes; acceptance criterion for every row is the hand-off standard from `planning-approach.md` §2: *the receiver can act without re-deriving my reasoning*.

| # | Story | Produces | Consumed by | Pattern / hire |
|---|---|---|---|---|
| S1 | **Claims audit of the three framing docs.** Every quantitative or evaluative claim in `industry-framing.md`, `personas-use-cases.md`, `api-connector-design-rationale.md` gets a row: claim, source (file + section or URL), status (verified / asserted / marketing-sourced / softened). I then fix or soften each unsupported claim in place. Runs **first**, so the debate in S2 argues over verified facts. | `decisions/claims-audit.md`; edits to the three docs | Team-lead (README/packet), 02 (`ai-workflow-narrative.md` cites the framing), evaluators | Desmond (Spreadsheet), 1–2 turns |
| S2 | **Structured written debate on the central framing** — is the password baseline a deliberate simplification the submission should narrate as "before/after", or a contradiction at a passkey company that the framing over-reads by claiming to know what evaluators "are really testing for"? Position → rebuttal → my synthesis; both positions preserved. `industry-framing.md` §2 and §4 revised to whatever survives. | `decisions/password-baseline-debate.md`; revised `industry-framing.md` | Team-lead (submission thesis), 02 (narrative alignment) | Imogen (Storyteller, position) vs Tobias (Designated Skeptic, rebuttal); Naomi synthesizes. 3 turns + lead |
| S3 | **Harden `api-connector-design-rationale.md` as the single source of truth for field/endpoint shape.** Add a closing "Contract summary for 03" section: the two entities with the assignment's exact field names; the `method` discriminator and 1-to-many profile→credential requirement (consistent with the 02/05 lookup-table decision); the endpoint list (`GET /profiles/{id}`, search endpoint with the `GET` query-param vs `POST /profiles/search` recommendation resolved, not left open); the provider-agnostic connector interface shape (`Authenticate`, `FetchIdentity`) with ABC/XYZ as adapters; and an explicit **"03 does not re-derive / out of scope for 03"** list (auth mechanism is 02's, schema modeling is 05's, address field names are fixed by the assignment). Then a newcomer's-question cold read; I fix everything the reader could not act on. | Revised `api-connector-design-rationale.md`; `decisions/rationale-cold-read.md` | **03** (reads before finalizing handlers and connector interface — soft dependency); 05 (§1 only) | Newcomer's question — reader borrowed from 04 (see §4), fallback a context-free haiku subagent. 1–2 turns |
| S4 | **Product requirements for search authorization** — a short appendix to `personas-use-cases.md` stating what Persona 2 legitimately needs from search (fields searchable: name, phone, partial match; masked-by-default results; deliberate expand with audit entry; no bulk export; pagination) and what Persona 2 must *not* be able to do, written as input to 02's adversarial-pair run on `profile:search` vs `profile:read:own`. Product requirements only — no scope names, token formats, or policy mechanics. | `personas-use-cases.md` "Search authorization — product requirements" appendix | **02** (input to its authz scoping decision), 03 indirectly via 02's hand-off | Rotating devil's advocate, 1 turn (seat: Tobias) |
| S5 | **UX notes for the two surfaces** that would sit on top of this backend — the admin profile-search console (Persona 2) and the onboarding pre-fill flow that triggers the connector (Persona 3). Text only, one page, describing screens and the one design rule each surface enforces (masked-by-default; never showing the partner password). Supports the narrative; no wireframes, no design canvas (see §3, waste). | `ux-notes.md` | Team-lead (packet), evaluators | Rotating devil's advocate, 1 turn (seat: Desmond — "which of these sentences is a claim?") |
| S6 | **Newcomer cold read of `industry-framing.md`** — evaluator-legibility test: can a reader with no project context state the thesis and the three seams in their own words after one pass? Fix what they could not. Secondary to S3; runs only if the borrowed reader has a second turn available. | Revised `industry-framing.md`; appended to `decisions/rationale-cold-read.md` | Evaluators, team-lead | Newcomer's question, 1 turn |
| T1 | **Track housekeeping** (Jira Task, not a Story): update `./CLAUDE.md` Status and AI-tooling note at a turn boundary (hires, patterns run, models used, research authorized); keep my `PLANNING.md` row current; append my three registry rows to `CASTING.md` after "hiring go"; create `decisions/` directory. | `./CLAUDE.md`, `../PLANNING.md` row, `../CASTING.md` rows | Team-lead, consistency pass | — |

Order: S1 → S2 → S3 → S4 → S5 → S6 (S6 optional). T1 runs alongside. Every pattern run ends in a file under `01-product-industry-research-design/decisions/` — chat is not the record.

## 2. Hire roster (3 hires — matches the suggested cast; I am first in arrival order, so the full palette is available)

All three: `model: sonnet`, read-only tools (Read, Grep, Glob, Bash), no file ownership — they return prose to me and I write the artifacts. No first name collides with a lead (Dana, Naomi, Marcus, Renata, Theo, Priya) or with each other. Roster spans both temperament axes (serious↔playful, cautious↔bold). Imogen supplies one of the org's required `playful + bold` seats.

### 2.1 Imogen Hale — Storyteller — function: narrative counterpart and framing proponent

- **Archetype:** Storyteller. **Tags:** `playful`, `bold`. **Slug:** `imogen-hale`.
- **Function:** holds and argues the framing position in S2 so I can synthesize rather than defend my own thesis; drafts the README-facing one-paragraph version of the thesis; rotating-advocate seat in S3's follow-up.
- **Patterns & role:** Structured written debate — *Position* (S2). Rotating devil's advocate — rotating *Objection* seat (S5 alternate).
- **Persona (CASTING.md §4 order, 138 words):**

> Warm, quick-to-metaphor product storyteller who opens a review by asking whose day this changes, and only then looks at the design. Decides by drafting the README paragraph first: if a feature's story can't be told in one paragraph to a support analyst, she treats the design as unfinished. Pushes back on framings only an insider would find persuasive, and on "the evaluators will get it" as a reason to skip the explanation. Eight years as a product manager on a consumer fintech onboarding team, then four writing developer-facing docs and launch narratives at a mid-size API company. Reads API contracts and sequence diagrams comfortably; does not write production code. Blind spot: charming beats correct — she will polish a narrative past the point where the facts still hold it up, so pair her with whoever asks for the number. In disagreement she restates the other side's story better than they told it, then says exactly where hers differs.

### 2.2 Desmond Okafor — Spreadsheet — function: claims auditor (my temperament opposite — required by variance rule 3)

- **Archetype:** Spreadsheet. **Tags:** `serious`, `cautious`. **Slug:** `desmond-okafor`.
- **Function:** audits every claim in the three docs for evidence (S1); rotating-advocate seat on the UX notes (S5); the standing "where is the number?" check on both S2 positions before I synthesize.
- **Patterns & role:** Claims audit (S1 — a one-hire review, not a catalog pattern; artifact is the table). Rotating devil's advocate — *Objection* seat (S5). Structured written debate — *pre-synthesis fact check*, 1 turn, not a debate participant.
- **Persona (147 words):**

> Dry, precise analyst who arrives with a table and distrusts any sentence carrying an adjective it can't cash. Quiet in a meeting until someone says "measurably" or "large market," then asks for the number, the source, and the date. Decides by keeping a two-column sheet — claim, evidence — and will not move a claim into a deliverable until the evidence column is filled or the claim is softened to what the evidence supports. Pushes back on narrative momentum, unsourced market sizing, and "everyone knows." Ten years in competitive-intelligence and pricing analysis at an enterprise software vendor, then three as a research analyst covering identity and access management at an industry-analyst firm. Treats vendor whitepapers as marketing until proven otherwise; fluent in SQL and spreadsheets, not a programmer. Blind spot: measures only what is measurable — he discounts true qualitative insight when it has no number, so pair him with the storyteller. Unemotional in disagreement; concedes instantly to a source, never to volume.

### 2.3 Tobias Lindqvist — Designated Skeptic — function: framing rebuttal and standing objection

- **Archetype:** Designated Skeptic. **Tags:** `serious`, `bold`. **Slug:** `tobias-lindqvist`.
- **Function:** writes the rebuttal in S2 (the "contradiction, not simplification" case, and the "you are projecting intent onto the evaluators" case); first Objection seat in the rotating-advocate runs (S4).
- **Patterns & role:** Structured written debate — *Rebuttal* (S2). Rotating devil's advocate — *Objection* seat (S4). Because his archetype's blind spot is objections without alternatives, every turn I give him ends with "and your counter-proposal is:".
- **Persona (149 words):**

> Courteous, contrarian product strategist who argues the other side by default and treats a room in agreement as a room that has stopped thinking. Decides late, after writing the strongest case against the leading option and finding it wanting; his recurring question is "what would have to be true for this to be wrong?" Pushes back on tidy framings, on any claim about what an evaluator "is really testing for," and on extension points built for futures nobody has committed to. Seven years as a strategy consultant to identity and payments vendors, then five as a principal product manager at an authentication company whose passwordless launch he argued against internally and then helped ship. Reads OAuth and WebAuthn specifications closely enough to catch a mischaracterization. Blind spot: objections without a counter-proposal — he can leave a decision weaker but not better, so the lead must ask for his alternative. In disagreement, precise and unhurried; changes his mind visibly when the argument is better.

### 2.4 Registry rows to append to `CASTING.md` §8 after "hiring go"

```
| imogen-hale | Imogen Hale | 01 | narrative counterpart / framing proponent | Storyteller | playful, bold | sonnet | structured written debate: position (S2); rotating advocate: objection (S5 alt) | 01-product-industry-research-design/profiles/imogen-hale/imogen-hale.md | .claude/agents/imogen-hale.md | queued |
| desmond-okafor | Desmond Okafor | 01 | claims auditor (lead's opposite) | Spreadsheet | serious, cautious | sonnet | claims audit (S1); rotating advocate: objection (S5); debate fact-check (S2) | 01-product-industry-research-design/profiles/desmond-okafor/desmond-okafor.md | .claude/agents/desmond-okafor.md | queued |
| tobias-lindqvist | Tobias Lindqvist | 01 | framing rebuttal / standing objection | Designated Skeptic | serious, bold | sonnet | structured written debate: rebuttal (S2); rotating advocate: objection (S4) | 01-product-industry-research-design/profiles/tobias-lindqvist/tobias-lindqvist.md | .claude/agents/tobias-lindqvist.md | queued |
```

Hire mechanics follow `CASTING.md` §6 exactly (text-first `write_profile.py --output file --assets tracked --root <this dir>`, `display.autostart: false`, image path pre-declared, no `CLAUDE.md` marker; agent def from `_TEMPLATE.md`; spawn with the fixed three first actions). Portrait prompts will be head-and-shoulders studio headshots in the same style as the leads', written into each persona's `generation` block; rendering is team-lead's queue, not mine.

## 3. Debate plan — which decisions get which pattern, and what is waste

**Runs (in order):**

1. **Claims audit** (S1, Desmond, 1–2 turns) — not a catalog pattern but the precondition for one: the S2 debate must argue over claims that have been checked. Artifact `decisions/claims-audit.md`.
2. **Structured written debate** (S2, 3 turns + synthesis) on the one decision that is genuinely mine and genuinely contestable: *is the password baseline a deliberate simplification to narrate as "before/after", or a contradiction at a passkey company, and does the framing over-reach by asserting what LoginID "is really testing for"?* Imogen writes the position (≤600 words), Tobias the rebuttal (≤600 words, ending with a counter-framing), Desmond a ≤200-word fact-check of both, then I synthesize. The synthesis is binding for `industry-framing.md` §2 and §4 and for the one-paragraph thesis the packet uses. Both positions stay in the record — LoginID is grading the reasoning, and a preserved dissent is better evidence of reasoning than a clean conclusion.
3. **Newcomer's question** (S3 primary, S6 secondary, 1–2 turns each) — the highest-ROI pattern in the catalog and the one that directly tests the hand-off standard. Primary target is `api-connector-design-rationale.md` because that is the file 03 has to *act* on; the framing doc is secondary because evaluators only have to *understand* it. Reader: 04's Newcomer hire, borrowed under the bounded two-party exception in `planning-approach.md` §2 (see §4). Fallback if the loan is not available when S3 is ready: a `haiku` subagent I spawn with **no project context except the file itself** — a genuinely cold reader, cheap, and within a lead's research-fan-out authority.
4. **Rotating devil's advocate** (S4, S5; 1 turn each; seat rotates Tobias → Desmond → Imogen so nobody becomes "the negative one") on the small product calls: the search-authorization product requirements (S4), the UX notes (S5), and — if there is a spare turn — whether `industry-framing.md` §3 (agentic identity / AAuth) earns its place at all or is scope creep dressed as context.

**Explicitly not debated (waste):**

- Anything the assignment text fixes: endpoint paths, JSON bodies, the `/identity` address field names, the two entities and their fields.
- The authn/authz *mechanism* for the API — 02's; I supply product requirements (S4) and stop.
- How `user_credential.method` is *modeled* — 05's, already coordinated with 02 as a lookup table; my doc asserts only that it is a real discriminator with room to grow.
- The OIDC `address`-claim observation in the rationale — that is a fact check (one `haiku`/low lookup of OpenID Connect Core 1.0 §5.1.1), not a debate.
- Red team/blue team, three hats, adversarial pair — built for security and engineering choices I do not own. Zero runs in this track.
- Wireframes, a design canvas, or any visual UX artifact — this is a backend assignment; `ux-notes.md` is text, one page, and stops there.
- Deep research to *decorate* the narrative (market-size figures, signup drop-off percentages). If a claim survives the audit only with a number nobody has, the fix is to soften the claim, not to fund a research pass.

**Deep research I pre-authorize (leads only, per `CASTING.md` §3):** exactly one item — verify the OIDC Core `address` claim member names (`street_address`, `locality`, `region`, `postal_code`, `country`) against the specification, because 03 may reuse those names in Go structs and the rationale currently asserts the match from memory. One `haiku`/low subagent, output to `research-oidc-address-claim.md` with the source URL, recorded in my `PLANNING.md` row. Any other request from a hire is decided against the test in §3: does the answer change a decision another track depends on? For this track, almost nothing does.

## 4. Dependencies

**Nothing hard-blocks on 01.** First wave, in parallel with 02 and 05 (`planning-approach.md` §2). My debates should complete before 03 finalizes handlers and the connector interface — that is a scheduling preference, not a block on 03's scaffolding.

**Provides:**

| To | File | Nature |
|---|---|---|
| 03 | `api-connector-design-rationale.md` (after S3) | **Soft dependency** — 03 reads it before finalizing handler shapes and the connector interface, not before starting scaffolding. It is the single source of truth for field/endpoint shape; 03 does not re-derive it. |
| 05 | `api-connector-design-rationale.md` §1 | Read-once check that "method as discriminator" and "1-to-many profile→credential" match `multi-db-strategy.md`. Already consistent with the 02/05 lookup-table decision. |
| 02 | `personas-use-cases.md` search-authorization appendix (after S4) | Product input to 02's adversarial-pair run on `profile:search` vs `profile:read:own`. |
| Team-lead / 02 | `industry-framing.md` (after S2), `decisions/password-baseline-debate.md` | The submission thesis the README and `ai-workflow-narrative.md` should tell consistently; the consistency pass checks this. |

**Needs:**

| From | What | Why | Timing |
|---|---|---|---|
| 02 (Marcus) | A one-message read-only confirmation that the product requirements I assert in rationale §2 (masked-by-default results, independent rate-limiting and pagination for search, every search attributable/auditable) are compatible with `api-auth-design.md` — or the specific change so my doc does not contradict his. | Two tracks must not disagree about what search returns. | Before S3 closes. |
| 02 (Marcus) | The 02→04 never-log list, when it lands. | `personas-use-cases.md` says "every search call is an audit-log event"; the appendix must not imply logging PII that 02 forbids logging. | Read when available; not blocking. |
| 04 (Theo) | Loan of his Newcomer hire for two cold reads (S3, S6): 2–4 turns total, after both tracks have "hiring go". Bounded two-party arrangement; the artifact lands in my `decisions/`. | The Newcomer archetype is the right reader and 01's three seats are spoken for. | When S3 is ready; fallback is the context-free `haiku` reader. |
| 05 (Priya) | Nothing beyond reading `multi-db-strategy.md` once to confirm §1 of my rationale matches her Go-shaped contract. | Avoid the "two tracks decide what `method` means" failure. | Read-only, whenever her contract is stable. |
| Team-lead | "hiring go" after variance review; registry tie-breaks if any. | Gate. | Now. |

## Verification (how I know this plan was executed)

- `01-product-industry-research-design/PLAN.md` is byte-identical to this plan (until Phase 3 revisions, which are dated).
- `../PLANNING.md` row 01 reads `plan drafted — awaiting hiring go`, then `hiring`, `debating`, `plan approved`, with hire counts updated at each step.
- After "hiring go": three files `profiles/{imogen-hale,desmond-okafor,tobias-lindqvist}/<slug>.md` exist, each `## Personality` 90–150 words; three `.claude/agents/<slug>.md` with `model: sonnet`; three `CASTING.md` §8 rows with `queued`; `grep -c "profile-gen:start"` across all `CLAUDE.md` files still totals 6.
- After debates: `decisions/claims-audit.md`, `decisions/password-baseline-debate.md` (position, rebuttal, fact-check, synthesis all present), `decisions/rationale-cold-read.md` exist; `api-connector-design-rationale.md` ends with a "Contract summary for 03" section including an explicit out-of-scope-for-03 list; `personas-use-cases.md` has the search-authorization appendix; `ux-notes.md` exists and is ≤1 page.
- `find . -name '*.go' -o -name '*.sql' -o -name 'go.mod' -o -name 'Dockerfile'` under this directory returns nothing, at every phase.
- `./CLAUDE.md` Status names the hires, the patterns run, the models used, and the one research pass authorized.
