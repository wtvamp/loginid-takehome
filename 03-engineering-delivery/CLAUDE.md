# Track: Engineering & Delivery

Refer back to `../CLAUDE.md` for the full assignment text and how this track fits with the other four.

## Mission

Write the actual code that answers all three assignment questions, in Go, implementing the designs the other tracks produce rather than re-deciding them here. Code does not need to be complete or working end-to-end — LoginID cares about the design and reasoning — but it should be structured and readable enough to demonstrate real engineering judgment.

## Owns

- Question 1: the DAO layer for `user_profile` (name, address, phone) and `user_credential` (username, method, password), written to support multiple backing databases (PostgreSQL/CockroachDB, SQLite, etc.) behind a common interface, per the strategy in `../05-data-ops/CLAUDE.md`.
- Question 2: the RESTful API service to search/retrieve `user_profile` data, wired to the authentication mechanism designed in `../02-ai-security-architecture/CLAUDE.md`.
- Question 3: the third-party IDP connector service implementing `/auth` and `/identity` against the contracts specified in the assignment, following the connector security design in `../02-ai-security-architecture/CLAUDE.md`.
- Project structure, module layout, and inline documentation of design decisions and trade-offs made during implementation.
- Test strategy (unit/integration scope, what's stubbed vs. real) — tests don't need full coverage, but the approach should be stated.

## Does not own

- Deciding the API auth mechanism or threat model — that's `../02-ai-security-architecture/CLAUDE.md`; implement what it specifies.
- Deciding the multi-database abstraction strategy or schema — that's `../05-data-ops/CLAUDE.md`; implement what it specifies.
- Deployment, containerization, CI/CD — that's `../04-infra-devops/CLAUDE.md`.
- Product/industry rationale for why the API is shaped this way — that's `../01-product-industry-research-design/CLAUDE.md`; reference it, don't re-derive it.

## Deliverables

- Go source implementing questions 1, 2, and 3.
- A short README in this directory explaining what's implemented, what's stubbed/mocked, and why, plus which AI tool/workflow was used to produce the code and how.
- Any test files demonstrating the stated test strategy.

## AI tooling note

Record which AI tool/workflow generated this track's code (this project is being built with Claude Code) and how much was AI-authored vs. human-reviewed/edited.

## Status

No implementation has started. This track's CLAUDE.md is scaffolding only — do not begin writing code until the product, security, infra, and data-ops tracks have their designs in place to implement against.

<!-- profile-gen:start slug=renata-cole -->
@profiles/renata-cole/renata-cole.md
<!-- profile-gen:end slug=renata-cole -->
