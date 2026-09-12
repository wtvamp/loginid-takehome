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

Phase 1 (org planning): `PLAN.md` in this directory is the track plan — eleven Stories, a four-hire roster (Adversary, Principled Architect, Tinkerer, Designated Skeptic), the debate plan, and dependencies; `../CASTING.md` §5 ratified with four ⟨02⟩ amendments; `planning-approach.md` gained Appendix A (attack-tree template) and Appendix B (harness constraints). Awaiting team-lead's "hiring go" before any persona or agent definition is created.

Next: available for `03-engineering-delivery` to implement against; open to review/adjustment as data-ops' schema and engineering's implementation surface any gaps.

<!-- profile-gen:start slug=marcus-ilori -->
@profiles/marcus-ilori/marcus-ilori.md
<!-- profile-gen:end slug=marcus-ilori -->
