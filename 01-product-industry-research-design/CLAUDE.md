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

Record which AI tool/workflow produced this track's output (this project is being built with Claude Code) and how — e.g., research done via a scoped subagent reading this file as its brief.

<!-- profile-gen:start slug=naomi-voss -->
@profiles/naomi-voss/naomi-voss.md
<!-- profile-gen:end slug=naomi-voss -->
