# Track: Infra & DevOps

Refer back to `../CLAUDE.md` for the full assignment text and how this track fits with the other four.

## Mission

Design how the services from the engineering track would actually be built, packaged, deployed, and operated. This is a design/documentation track for a take-home — it does not stand up real infrastructure, but it should be concrete enough that it clearly could.

## Owns

- Build and containerization approach for the DAO-backed API service and the IDP connector service (e.g., Dockerfiles, multi-stage Go builds).
- Environment/configuration strategy for switching the DAO between PostgreSQL/CockroachDB and SQLite (env vars, config files, or a database driver selection mechanism) — implements the storage-agnostic contract that `../05-data-ops/CLAUDE.md` designs.
- CI/CD pipeline design: what would run on a commit/PR (build, lint, test, security scan) for a project like this.
- Secrets injection mechanics at deploy time for whatever `../02-ai-security-architecture/CLAUDE.md` specifies needs to be a secret (DB credentials, third-party IDP credentials, signing keys) — this track implements the delivery mechanism, security architecture sets the requirement.
- Observability approach: logging, metrics, and tracing considerations for the API and the connector, especially around what must never be logged (raw passwords, tokens, PII) per the security track's guidance.
- Local dev-loop story: how a developer would run this stack locally (e.g., docker-compose with a local Postgres and the services).

## Does not own

- Application code itself — that's `../03-engineering-delivery/CLAUDE.md`.
- What counts as a secret and why, or the auth/threat model — that's `../02-ai-security-architecture/CLAUDE.md`; this track only implements the delivery of it.
- Database schema/migration content — that's `../05-data-ops/CLAUDE.md`; this track only covers how migrations would run in a pipeline, not their content.

## Deliverables

- Containerization design (Dockerfile sketch or description) for both services.
- A CI/CD pipeline description or config sketch.
- A local dev-loop description (e.g., docker-compose sketch).
- A short note on observability and what must be excluded from logs.

## AI tooling note

Record which AI tool/workflow was used to produce this track's design output and how.

## Status

No infra work has started. Do not begin until the engineering track's service boundaries are defined enough to containerize and deploy against.

<!-- profile-gen:start slug=theo-bergman -->
@profiles/theo-bergman/theo-bergman.md
<!-- profile-gen:end slug=theo-bergman -->
