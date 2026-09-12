# Track 02 Plan — AI Architecture & Security Architecture (Phase 1)

Owner: Marcus Ilori. Written in plan mode per `PLAN.md` Phase 1. Once approved it is copied verbatim to `02-ai-security-architecture/PLAN.md`; the `~/.claude/plans/` copy is scratch.

## Context

The org plan (`PLAN.md`) requires every lead to plan in plan mode with four mandatory sections before hiring. Track 02 carries two extra obligations as the AI-architecture owner: ratify or amend the orchestration-pattern catalog in `CASTING.md` §5, and check the harness facts in `PLAN.md` "Verified mechanics" against `research-claude-architecture-best-practices.md` before other leads build on them. The four first-wave deliverables of this track (`threat-model.md`, `api-auth-design.md`, `connector-security.md`, `ai-workflow-narrative.md`) are complete; this plan turns them into debated, second-reviewed, receiver-shaped outputs and adds the org-layer story to the AI narrative. No code anywhere in this plan.

## 0. Ratification of `CASTING.md` §5 and the mechanics check

**Verdict on §5: ratified, with four amendments.** The six protocols are sound and their use/waste assignments match my own planning-approach reasoning. The amendments each target a structural weakness that the palette's own blind-spot column predicts:

1. **Red team / blue team** — add a mandatory ranking step: the Adversary hands over a *top-five* ranked list of leaves before the blue seat responds (counters "sees threats everywhere, no prioritization" structurally rather than by exhortation). Add an optional one-turn *alternatives* seat for the Tinkerer between red and blue: propose design changes that delete leaves outright — a leaf that cannot exist beats a control. Add a residual-risk scoring rule (below). Point to the attack-tree template in `planning-approach.md` Appendix A.
2. **Adversarial pair** — every numbered objection from the Designated Skeptic must end with a one-line "what would change my mind" (counters "objections without a counter-proposal"). The Proposer must answer every objection in writing; it may decline to revise, but not silently.
3. **Structured written debate** — the lead's synthesis must state which position was stronger on which point and why; preserving both positions is necessary but not a substitute for a ruling (counters the Consensus Weaver blind spot, which is the PM's own listed archetype).
4. **All patterns** — each decision artifact records, in a footer, the model each seat ran on and the number of hire turns consumed, so `ai-workflow-narrative.md` can cite measured cost rather than intent.

Where the text lives, one source of truth per thing: the four protocol amendments go into `CASTING.md` §5 in place (my permitted scope); the attack-tree *template* — a table shape, not a protocol — goes into `planning-approach.md` as **Appendix A** with a one-line pointer from §5. No new document.

**Attack-tree template (Appendix A content):** columns `# | Leaf (attack path toward the goal) | Precondition (what the attacker must already have) | Likelihood L/M/H | Impact L/M/H | Blue response: Control or Accepted risk | Owner (02 requirement / 03 implements / 04 mechanism) | Residual L/M/H | Source`. Rules: red ranks the top five first; blue answers every leaf, and an "accepted risk" must say why it is acceptable *for this system*, not in general; the lead assigns Residual with a one-line justification; any Residual **H** must produce either a follow-up Story or an accepted risk visible to Warren in the artifact. The completed table is appended to the surface's design doc (`connector-security.md`); the full positions live in `decisions/<topic>.md`.

**Mechanics check against the research doc: no contradictions.** Three notes for the record, to be added as a short "Harness constraints" addendum to `planning-approach.md` so it stays the single source of truth for the tiers:

- Effort: the harness has no per-agent effort key; teammates inherit `high`, and `modelSettings.haiku.effortLevel: low` is the only per-model lever. My sonnet-medium tier therefore collapses to sonnet-high (accepted — errs upward on the tracks that matter), and *sonnet/low research fan-out is unavailable*: research is haiku/low, or sonnet/high when judgment is needed, which is costlier than planned and should be used sparingly.
- The per-agent `PreToolUse` hook and the `.claude/agents` frontmatter facts are outside my research doc's scope; I have no primary-source basis to dispute them and record them as verified by team-lead, not by 02.
- Research doc §4 (prompt caching) still applies one level down: a hire's `.claude/agents/<slug>.md` is part of that hire's stable prefix. Do not edit a hire's definition while it is running; edit at spawn boundaries.

## 1. Deliverables and tasks (Epic: *02 AI & Security Architecture*; one Story each)

| # | Story | Hand-off produced (file) | Consumer | Depends on |
|---|---|---|---|---|
| S1 | Ratify/amend `CASTING.md` §5 (four amendments above) and add Appendix A to `planning-approach.md` | `CASTING.md` §5; `planning-approach.md` App. A | all leads, before any red/blue runs | — |
| S2 | "Harness constraints" addendum to `planning-approach.md` | `planning-approach.md` | all leads; `ai-workflow-narrative.md` | — |
| S3 | Red/blue on the connector token lifecycle → attack-tree table with residual scores | `connector-security.md` (table appended); `decisions/connector-token-lifecycle-redblue.md` (full record) | 03's opus token-lifecycle seat; 04 (any control that is a mechanism) | hires; S1 |
| S4 | Adversarial pair on search-API authorization scoping | `decisions/search-authz-scoping.md`; `api-auth-design.md` gains "Objections considered" | 03 (middleware) | hires |
| S5 | Structured written debate on the core claim of `ai-workflow-narrative.md` | `decisions/ai-workflow-claim-debate.md` | S9 | hires |
| S6 | **02→03 receiver-shaped auth hand-off** — token scheme (client-credentials → JWT, asymmetric signing, claim set `sub/scope/exp/iat/aud`, TTL 5–15 min, no refresh, no denylist), **where validated** (middleware at the API edge; key retrieval), the final scope vocabulary and object-level policy hook from S4, pagination ceiling and per-client rate limit as requirements, audit-log field list, and an explicit "not yours to decide" list | `handoff-03-auth.md` | 03 — Renata scaffolds now, needs this before middleware is finalized | S4 |
| S7 | **02→04 secrets inventory + never-log list** — table `secret | used by (API / connector / authz server) | injection requirement | rotation expectation | never-log`; never-log list (vendor password, vendor token, PII field values, raw vendor error bodies, JWT signing private key, client secrets). Assumes **Kubernetes-native Secret delivery on the LAN lab cluster** 04 names as the design-only target (Warren's decision via Theo, `LOG.md`), stated explicitly rather than a generic "managed store"; requirements still hold if the mechanism changes | `handoff-04-secrets.md` | 04 — gates Theo's producer phase | one Skeptic objection turn; no debate |
| S8 | Independent second-review pass on the security-graded docs (`threat-model.md`, `api-auth-design.md`, `connector-security.md`) by hires who did not author or blue-team them | `decisions/second-review-security-docs.md`; fixes applied in place | Warren / final consistency pass | S3, S4 |
| S9 | Revise `ai-workflow-narrative.md` to cover the org layer — personas, patterns, debates, measured turn counts and models from the decision-artifact footers; incorporate the S5 ruling honestly, whichever way it goes; state plainly that a real external agent (the cluster operator 04 describes) exists in the environment and was deliberately kept out of scope — never contacted, nothing deployed | `ai-workflow-narrative.md` | submission reader | S3–S5 |
| S10 | `threat-model.md` maintenance: (a) repoint the stale reference to `05-data-ops/schema/postgres_cockroachdb.sql` (wiped) at the prose schema in `../05-data-ops/multi-db-strategy.md` — verified 2026-09-12 that `auth_method` and `secret / hash_algo / hash_cost` are already there in prose; (b) add one trust-boundary line: an **external operator agent applying manifests to the deployment target** (04's design-only lab cluster) is a privileged actor outside this service — whoever can apply a manifest can read any Secret it mounts — so the assumption "secrets are safe once in the cluster" is named, not silent | `threat-model.md` | 04 | none — can run at Phase 3 start |
| S11 | Post-implementation independent review of 03's auth middleware and connector token code (security-graded surfaces only) | `decisions/second-review-implementation.md` | 03 | Warren's code go-ahead — Phase 4+, listed now so it is not forgotten |
| T1 (Task) | Deep research, pre-authorized for one topic only if the Adversary or Architect requests it: bearer-token handling and OAuth 2.0 security best current practice (RFC 6750, RFC 9700) to arm S3 with primary sources; haiku/low fan-out run by me | `research-oauth-token-handling.md` | S3 | request from a hire |

Cross-team asks (not my stories): 04's Newcomer performs the cold read of `handoff-03-auth.md` and `handoff-04-secrets.md` (already the one pattern `CASTING.md` assigns 04); the red/blue on the connector is run **once**, here, with 03's opus seat reading the artifact rather than 03 running a second one — I will flag this to Renata so it is not duplicated.

## 2. Hire roster (4; all sonnet; all read-only per `_TEMPLATE.md` — they return prose, I write the artifacts)

No first name collides with Dana, Naomi, Marcus, Renata, Theo, or Priya. Roster spans serious↔playful and cautious↔bold; two hires are `playful, bold`, contributing to the org-wide minimum of three.

### H1 — Tomasz Wrede · Adversary · `playful, bold` · function: attack-tree author
Patterns: red/blue (red seat, S3); optional one-turn pre-read on S4 ("what would I do with each scope"). Requester for T1.

> Mischievous and quick — the reviewer who reads a sequence diagram and immediately asks where he would stand to steal from it, and grins while asking. Decides by enumerating attack paths first and only then asking which ones matter, which he admits is backwards. Pushes back hardest on any claim that a control is "sufficient" until someone names the attacker it stops and the one it does not. Eight years on an internal red team at a large retailer, then four running penetration tests against payment and identity APIs for a consultancy; fluent in token theft, replay, and cache-poisoning paths, less so in formal proofs. Blind spot: he sees threats everywhere and ranks them poorly — left alone he produces forty leaves of equal weight, so make him hand over a top five before anyone answers. Argues cheerfully, concedes fast when a leaf is shown unreachable, and keeps a list of the ones he lost.

### H2 — Helena Marsh · Principled Architect · `serious, cautious` · function: controls author and standards reviewer
Patterns: red/blue (blue seat, S3); adversarial pair (Proposer, S4); second review of `threat-model.md` and `api-auth-design.md` (S8 — she did not author them and is not blue on them).

> Measured and exact; in a meeting she speaks last, usually with a section number. Decides from the threat model downward and writes every decision as a record with the alternatives she rejected and why. Pushes back on any control offered without the attack it defeats, on any trade of a security property for developer convenience, and on a citation that points at a blog when a specification exists. Eleven years across a national bank's identity platform and a government PKI programme; deep in OAuth 2.0 and its security best current practice, JOSE and the ways JWT libraries fail, and TLS deployment. Blind spot: over-specification — she designs for threats this take-home does not have and can mistake thoroughness for finished, so pair her with someone asking what the assignment actually needs. Courteous under challenge, unhurried, and changes position when shown evidence, saying so plainly.

### H3 — Felix Adebayo · Tinkerer (silly & innovative) · `playful, bold` · function: alternatives generator — **my temperament opposite (required)**
Patterns: red/blue (alternatives seat between red and blue, S3 — "what if the connector held no token at all?"); structured written debate (Position A, S5).

> Playful and fast-associating; he describes a bearer token as a hotel keycard that opens every door until Thursday, and means it as a design question. Decides by sketching three alternatives, one deliberately strange, before defending any, and drops his own without ceremony when someone shows a cleaner failure mode. Pushes back on "that is the standard approach" whenever nobody in the room can say what the standard protects against, and on any diagram with a box labelled "just cache it". Five years building fraud-scoring pipelines at a payments startup, three at a mid-sized identity vendor prototyping passkey flows; Go and Rust, and an unreasonable fondness for property-based tests. Blind spot: novelty bias — he underweights boring, proven controls and needs someone to cite the RFC at him. Warm in disagreement, never sarcastic, delighted to be wrong in an interesting way.

### H4 — Ingrid Solano · Designated Skeptic · `serious, bold` · function: standing objector and independent reviewer
Patterns: adversarial pair (Skeptic, S4); structured written debate (Position B, S5); one objection turn on the secrets inventory (S7); second review of `connector-security.md` (S8 — she is not blue on it).

> Dry and direct; in a meeting she is the one asking "compared to what?" before the proposal is finished. Decides only after hearing the strongest case against the room's favourite, and makes that case herself if nobody else will, even when she privately agrees. Pushes back on consensus that arrived too quickly, on scope vocabularies invented before anyone listed the callers, and on evidence that is one number from one vendor. Nine years as a security reviewer on an enterprise SaaS platform, then three chairing design reviews at a healthcare data company where every authorization decision met a regulator. Background in authorization models — RBAC, ABAC, object-level policy — and in reading audit logs after something went wrong. Blind spot: objections without a counter-proposal — she can leave a team knowing what is wrong and not what to do, so every objection carries a "what would change my mind" line. Polite, unyielding on process, quick to record when an objection was answered.

Registry rows (appended to `CASTING.md` §8 only after "hiring go"): `tomasz-wrede`, `helena-marsh`, `felix-adebayo`, `ingrid-solano`; Track 02; Model sonnet; Persona `02-ai-security-architecture/profiles/<slug>/<slug>.md`; Agent def `.claude/agents/<slug>.md`; Portrait `queued`.

## 3. Debate plan

| Decision | Pattern | Seats | Turns | Artifact |
|---|---|---|---|---|
| Connector token lifecycle: TTL, at-rest encryption, per-vendor keying, fail-closed, "hold no token" alternative | Red/blue (amended) | Tomasz red → Felix alternatives → Helena blue → Tomasz one rebuttal on the top three → Marcus scores residual | 4 + lead | `decisions/connector-token-lifecycle-redblue.md`; table into `connector-security.md` |
| Search-API authz scoping: `profile:search` vs `profile:read:own` vs `profile:read:any`; object-level policy; who gets `profile:search` at all | Adversarial pair (amended) | Helena proposes → Ingrid numbered objections with "what would change my mind" → Helena answers/revises → Marcus rules (optional Tomasz pre-read, 1 turn) | 3 (+1) + lead | `decisions/search-authz-scoping.md`; "Objections considered" into `api-auth-design.md`; feeds S6 |
| Core claim of `ai-workflow-narrative.md` — does the multi-agent, persona-driven workflow produce *better design* than one strong session, or is it cost without evidence? (Anthropic's 90.2% figure is for research breadth, not design quality) | Structured written debate (amended) | Felix: it does, and the narrative undersells it → Ingrid: unproven; keep the narrow claim → Marcus rules per point, and the ruling goes into the narrative whichever way it lands | 3 + lead | `decisions/ai-workflow-claim-debate.md` |
| Secrets inventory completeness | Single objection turn | Ingrid: what is missing / mis-owned | 1 | inline in `handoff-04-secrets.md` |
| Security-graded docs, independent read | Second review | Helena on threat model + API auth; Ingrid on connector security | 2 | `decisions/second-review-security-docs.md` |

Budget: about 14 hire turns for the whole track, all sonnet/high. **Declared waste — not debated:** the Argon2id hashing decision (settled by OWASP/NIST — cite, do not debate); the OAuth2 grant *type* (already reasoned in `api-auth-design.md`; S4 is about scoping, not the grant); STRIDE as the framework; endpoint paths and JSON bodies (fixed by the assignment); anything about DAO CRUD or Dockerfiles; a three-hats run (no decision in this track has three legitimate optimist/pessimist/pragmatist positions). No pattern gets a second run "to be sure".

## 4. Dependencies

**From 05 (Priya):** the `auth_method` lookup-table shape (`id, name, requires_secret, is_active`; `user_credential.secret / hash_algo / hash_cost`) — **settled** by direct two-party coordination and confirmed present in prose in `multi-db-strategy.md`; my threat model is already method-aware. Nothing further needed from 05 for this track's Phase 3; the Go-shaped contract she is delivering is 03's dependency, not mine.

**From 01 (Naomi):** `api-connector-design-rationale.md` as the single source of truth for field/endpoint shape — a soft dependency, read before finalizing S6, not before starting.

**To 03 (Renata):** `handoff-03-auth.md` (S6) — she scaffolds now and needs it before middleware finalization; target: immediately after S4 rules, early Phase 3. The red/blue table (S3) is the requirement set for her opus token-lifecycle seat; target mid Phase 3. One red/blue, run here, read there.

**To 04 (Theo):** `handoff-04-secrets.md` (S7) — gates his producer phase; it depends on no debate, only one objection turn, so it is the **first** deliverable out after hiring. His Newcomer cold-reads both hand-offs.

**From 04 / Warren (via team-lead, recorded in `LOG.md`):** the deployment target is a real Kubernetes lab cluster on the LAN operated by an external agent, used as a **design-only** reference. This track assumes Kubernetes-native Secret delivery in S7 and names the operator as a trust boundary in S10. Hard rule inherited: no action on that cluster and no contact with its operator agent, ever, without Warren's explicit go — this track has no reason to touch either.

**To all leads:** `CASTING.md` §5 amendments and Appendix A (S1) before any lead runs a red/blue; the harness-constraints addendum (S2) before anyone spawns a "sonnet/low" research subagent that will actually run at high.

**From team-lead:** "hiring go" — nothing in §2 is created or spawned before it.

## Files touched after ExitPlanMode (Phase 1 close-out only)

1. Copy this plan → `02-ai-security-architecture/PLAN.md`.
2. `PLANNING.md` — my row only → `plan drafted — awaiting hiring go`, hires `4 / 0 / 0`, planning notes pointer, date.
3. `CASTING.md` §5 — the four amendments in place, nothing outside §5.
4. `planning-approach.md` — Appendix A (attack-tree template) and the Harness-constraints addendum.
5. `02-ai-security-architecture/CLAUDE.md` Status — one line pointing at `PLAN.md` (turn-boundary edit).
6. ≤200-word summary with roster to team-lead.

Not touched until "hiring go": `profiles/`, `.claude/agents/`, `CASTING.md` §8, any spawn.

## Verification

- `diff ~/.claude/plans/ethereal-whistling-valley.md 02-ai-security-architecture/PLAN.md` is empty.
- `PLANNING.md` row 02 reads `plan drafted — awaiting hiring go`; no other row changed.
- `git diff CASTING.md` shows changes only between the `## 5.` and `## 6.` headings.
- `planning-approach.md` contains "Appendix A" and "Harness constraints".
- Roster check against `CASTING.md` §2: four distinct archetypes, includes Tinkerer (lead's opposite), spans both axes, two `playful, bold`; no first-name collision with any lead.
- `find 02-ai-security-architecture -name '*.go' -o -name '*.sql'` returns nothing; no files under `profiles/` beyond `marcus-ilori/`; `.claude/agents/` holds only `_TEMPLATE.md` from this track's side.
