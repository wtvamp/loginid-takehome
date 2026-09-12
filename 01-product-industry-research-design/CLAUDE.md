# Track: Product & Industry Research (incl. Design)

Refer back to `../CLAUDE.md` for the full assignment text and how this track fits with the other four.

## Mission

Frame the three assignment questions as a real product problem, not just three isolated coding exercises. Show industry awareness of the space LoginID operates in — identity, authentication, and credential/PII management — and use that framing to justify the design choices the engineering track implements.

## Owns

- Industry/competitive framing: where this kind of DAO + REST API + third-party IDP connector fits in the identity/IAM landscape (e.g., how it compares to what LoginID itself does with passkeys/FIDO2, and why a password-and-username-based `user_credential` model is the deliberately simpler baseline the assignment is asking for).
- Personas and use cases: who calls this API, why they need to search/retrieve user profile data, and why a service would need to pull PII from a third-party identity provider rather than owning that data itself.
- API and data contract design rationale from a product lens: what fields belong in `user_profile` vs `user_credential`, what "search/retrieve" needs to support, and why the `/auth` and `/identity` connector contracts are shaped the way LoginID specified them.
- Any UX/design artifacts for surfaces that would sit on top of this (e.g., an admin console for profile search, an onboarding flow that triggers the IDP connector) — light-touch, this is a backend assignment, so design work here should support the narrative rather than compete with it for attention.

## Does not own

- Actual security threat modeling and authn/authz mechanics for the API — that's `../02-ai-security-architecture/CLAUDE.md`.
- The Go implementation — that's `../03-engineering-delivery/CLAUDE.md`.
- Database schema and multi-DB strategy — that's `../05-data-ops/CLAUDE.md`.

## Deliverables

- A short industry/competitive framing document.
- Persona and use-case notes tied directly back to the three assignment questions.
- Design rationale for the API surface and the third-party connector contracts, written so the engineering track can implement against it without re-deriving the reasoning.

## AI tooling note

Produced with Claude Code, running as an org of standing sessions: this track's lead (persona Naomi Voss, Sonnet 5, top-level session) hired three sub-teammates via the Agent tool — Imogen Hale (Storyteller), Desmond Okafor (Spreadsheet, lead's temperament opposite), Tobias Lindqvist (Designated Skeptic), all `model: sonnet`, read-only tools, no file ownership (they return prose, the lead writes artifacts). Patterns run: a one-hire claims audit (Desmond, S1); a structured written debate — position (Imogen) / rebuttal (Tobias) / fact-check (Desmond) / synthesis (lead) — on the password-baseline framing (S2); a newcomer's-question cold read (Wesley Okonkwo, borrowed from track 04 under a bounded two-party loan) on the connector rationale hand-off to track 03 (S3); rotating devil's-advocate objection seats (Tobias on search-authorization requirements, S4; Desmond on the UX notes, S5). One `haiku`-tier deep-research pass, lead-authorized: verifying the OIDC Core `address` claim member names against the specification (`research-oidc-address-claim.md`). Every pattern run produced a file under `decisions/` — chat signaled readiness, it never carried content.

## Status

Design phase and Phase 1 planning complete; hiring go granted 2026-09-12. All three hires spawned as standing teammates. S1–S5 of `PLAN.md` complete: `decisions/claims-audit.md` (claims audit, two claims softened/one strengthened in `industry-framing.md` and `personas-use-cases.md`); `decisions/password-baseline-debate.md` (structured written debate — synthesis dropped the "LoginID designed this gap on purpose" claim as unfalsifiable, kept the extension-points argument reframed as industry-standard good practice, `industry-framing.md` §2/§4 revised accordingly); `api-connector-design-rationale.md` hardened with a "Contract summary for 03" section (search resolved as `POST /profiles/search`, `expand: bool` shape fixed, explicit out-of-scope-for-03 list) and `decisions/rationale-cold-read.md` (Wesley Okonkwo's cold read, one ambiguity fixed); `personas-use-cases.md` gained a "Search authorization — product requirements" appendix (Tobias's objection added two lines on volume-limiting and query-specificity-independent masking); `ux-notes.md` written (two surfaces, one page, two claim-like sentences caught by Desmond's rotating-advocate pass and softened to intent statements). S6 complete: Wesley's second cold read correctly restated the thesis and the post-S2 before/after-vs-author-intent distinction with no fixes needed (`decisions/rationale-cold-read.md`); the 04 loan closed at 2 of 2–4 turns. All six PLAN.md stories done.

<!-- profile-gen:start slug=naomi-voss -->
@profiles/naomi-voss/naomi-voss.md
<!-- profile-gen:end slug=naomi-voss -->
