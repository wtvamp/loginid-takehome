# Track: AI Architecture & Security Architecture

Refer back to `../CLAUDE.md` for the full assignment text and how this track fits with the other four.

## Mission

Own two things that are related but distinct: (1) the story of how AI was used as an architecture and delivery tool across this whole submission, and (2) the security architecture for everything the engineering track builds — the DAO, the REST API, and the third-party IDP connector.

LoginID explicitly asked submitters to describe any tool, framework, or AI used — this track is where that answer lives in full, even though other tracks should each note their own AI usage briefly too.

## Owns

- AI architecture narrative: what Claude Code was used for across the project, how the five-track structure itself was designed and orchestrated (root CLAUDE.md + per-track CLAUDE.md + scoped subagents), what was AI-generated vs. human-directed, and why that workflow was chosen.
- Threat modeling for the system: attacker-relevant risks around storing `user_credential` (usernames/passwords), storing and serving `user_profile` PII, and the third-party connector holding vendor `access_token`s.
- API authentication/authorization design for the REST service in question 2 — what "security API authentication" means here (e.g., API keys, OAuth2 client credentials, mTLS, or JWT bearer tokens), and why.
- Credential handling: password storage (hashing/salting strategy), token lifecycle for third-party `access_token`s from question 3, and secrets management for any credentials this service itself needs to call ABC/XYZ.
- Security review of the connector contracts in question 3: what happens to the third-party `access_token` once retrieved, how `/identity` calls are authenticated to the vendor, and what this service should and shouldn't log or persist from a PII/security standpoint.

## Does not own

- The industry/product framing for why this system exists — that's `../01-product-industry-research-design/CLAUDE.md`.
- Writing the Go code itself — that's `../03-engineering-delivery/CLAUDE.md`, though this track's design should be specific enough to implement directly.
- Runtime secrets storage mechanics in a deployed environment (e.g., which secrets manager, which CI/CD injects what) — that's `../04-infra-devops/CLAUDE.md`; this track sets the requirement, infra/devops implements it.
- Data retention/governance policy for stored PII — that's `../05-data-ops/CLAUDE.md`; this track covers security of access and transit, not lifecycle/retention.

## Deliverables

- A written AI-tooling/workflow summary for the whole project.
- A threat model covering credential storage, PII exposure, and third-party token handling.
- A concrete auth/authz design for the REST API in question 2.
- A security design for the IDP connector in question 3, including token handling and what gets logged.

## AI tooling note

This track is itself where the project's AI-usage story is documented in depth — treat it as both a subject-matter track and the place other tracks' brief AI notes get rolled up if useful. See `ai-workflow-narrative.md` for the full write-up.

## Status

Threat model, API auth/authz design, and connector security design are drafted:

- `threat-model.md` — STRIDE-style analysis of `user_credential`, `user_profile`, and third-party `access_token` exposure, per-asset mitigations.
- `api-auth-design.md` — OAuth2 client-credentials grant with short-lived JWT bearer tokens and scoped, object-level authorization for question 2's search/retrieve API. Reasoned against static API keys and mTLS-only alternatives.
- `connector-security.md` — token handling, vendor-call authentication, and logging/persistence rules for question 3's `/auth` and `/identity` connector.
- `ai-workflow-narrative.md` — the full AI-tooling/workflow story for the submission, building on `research-claude-architecture-best-practices.md`.
- `planning-approach.md` — team-wide recommendation (requested by team-lead) for model/effort pairings per role, planning-phase coordination pattern, cross-track dependency order, and the shape/location of the shared planning artifact. Synthesized from all four other tracks' input plus this track's own view. Two follow-ups for this track surfaced from it, deferred until the planning pause lifts: a receiver-shaped 02→03 hand-off (token scheme and validation point, no alternatives discussion) and a 02→04 secrets inventory plus never-log list — the decisions exist in `api-auth-design.md` / `connector-security.md` but not yet in that form.

Coordinated with `../05-data-ops/CLAUDE.md` on `user_credential.method` being modeled as a lookup table (not a native enum) — the credential-storage design in `threat-model.md` is method-aware as a result (password vs. WebAuthn vs. TOTP each have different storage requirements), not a single password-hashing assumption.

Phase 1 (org planning): `PLAN.md` in this directory is the track plan — eleven Stories, a four-hire roster (Adversary, Principled Architect, Tinkerer, Designated Skeptic), the debate plan, and dependencies; `../CASTING.md` §5 ratified with four ⟨02⟩ amendments; `planning-approach.md` gained Appendix A (attack-tree template) and Appendix B (harness constraints).

Phase 2 (execution): all four hires spawned and live as named teammates in this session. Completed: **S7** — `handoff-04-secrets.md` v1, Ingrid Solano's objection turn folded in (four objections, all accepted), sent to and cold-read by 04's Newcomer, two gating items fixed. **S3** — red/blue on the connector token lifecycle (Tomasz Wrede red, Felix Adebayo alternatives, Helena Marsh blue, Tomasz rebuttal, Marcus residual scoring); `connector-security.md` §1 rewritten to a no-cache-by-default posture with two hardened implementation controls (fresh auth header per request, explicit fetch-use-zeroize boundary); attack-tree table appended; full record in `decisions/connector-token-lifecycle-redblue.md`; residual all L except one accepted M (bearer-token replay, no vendor PoP lever), no follow-up Story required; handed to 03's opus token-lifecycle seat. **S4** — adversarial pair on search-API authz scoping (Helena proposes, Ingrid five numbered objections, Helena revises all five, Marcus rules); `api-auth-design.md` gained a finalized scope vocabulary, provisioning bars, object-level policy hook, decomposition-resistant rate limiting, and lifecycle-triggered revocation; full record in `decisions/search-authz-scoping.md`. **S6** — `handoff-03-auth.md` written and sent to Renata Cole (03): token scheme, validation point, S4's scope vocabulary, audit-log fields, "not yours to decide" list.

**All eleven Stories complete except S11** (post-implementation review, deferred to Warren's code go-ahead). S5 (`decisions/ai-workflow-claim-debate.md`) ruled against a general "multi-agent beats solo" claim and for a narrower, mechanism-level one, with the losing arguments preserved honestly. S8 (`decisions/second-review-security-docs.md`) required a mid-run reassignment when Helena's S4/S3 authorship broke her original independence on two of the three documents — caught and fixed before running, not after; ten accepted findings resulted, the sharpest being a decomposition-resistant rate-limit control from S4 that was a silent no-op for one of its two target scopes. S9 folded the org layer — personas, patterns, measured turn/model footers, S5's ruling stated plainly including the side that lost, the S8 reassignment as a general lesson ("independence is checked at review time, not assigned once"), and the real external operator agent named as deliberately out of scope — into `ai-workflow-narrative.md`. S10's two required fixes (repoint the wiped-schema reference, name the operator trust boundary) were already present in `threat-model.md` from earlier work; verified clean, no further edit needed.

Next: available for cross-track consistency pass; open to review/adjustment as 03's and 05's implementation surfaces any gaps in the S3/S4/S6 hand-offs.

**Phase 4 — consistency-pass fixes (2026-09-12):** the cross-track consistency pass (`../decisions/cross-track-consistency.md`) named this track owner on 15 findings; all fixed in place, none requiring a debate reopened. Headline fixes: F3 (the vendor token had no consistent home — `connector-security.md` §1 now states our `/auth` returns it to the authenticated internal caller, which presents it as a header on the following `/identity` call; connector stays stateless); F13 (signing-key custody contradiction — the issuer is now a second Deployment of the same `api-service` image with its own ServiceAccount, not RBAC alone on a shared mount); F-pag (corrected the pagination reasoning against 05's accepted `Offset int` contract rather than reopening it); F39–F42 (this track's own narrative and decision records undercounted their own evidence — fixed to report the tallies honestly, including where the correction strengthens rather than weakens the pattern's case). Two new backlog stories added (`backlog.md` S12, S13, `from-consistency-pass`) for the genuine new implementation surface F13/F3/F15 created. Bounded two-party sentences sent directly to Naomi (F3), Theo (F13), Renata (F50); Priya confirmed F-pag needs no change to her contract. `PLANNING.md` row set to `plan approved`.

<!-- profile-gen:start slug=marcus-ilori -->
@profiles/marcus-ilori/marcus-ilori.md
<!-- profile-gen:end slug=marcus-ilori -->
