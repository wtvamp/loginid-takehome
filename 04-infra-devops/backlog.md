# Track 04 — Infra & DevOps Backlog

Track: 04-infra-devops. Lead: Theo Bergman. Epic: "Infra & DevOps: Containerization, CI/CD, and Operations" (key: TBD, team-lead fills in on transcription).

Authored now per Phase 4; will be re-checked against the consistency pass's cross-cutting findings (`PLAN.md` §"Cross-cutting — enforcement sites") once that report lands, particularly for Story 6 below. All stories carry `no-code-yet` — Warren's Phase 4 go covers backlog authoring, not implementation; a separate, explicit code go-ahead is still required before any `.go`, Dockerfile, CI config, or compose file is written for real.

## Story 1 — Containerize `api-service` and `idp-connector`

**Description:** Build the two multi-stage Go images (distroless final stage, non-root, read-only root filesystem) and the Deployment/Service/NetworkPolicy manifests targeting the lab cluster, per `./containerization-design.md`. Covers both binaries' Dockerfiles, resource requests/limits sized per service (CPU-bound `api-service` vs. I/O-bound `idp-connector`), the migration `Job` (parallelism 1, gated by the CI pipeline's exclusivity mechanism, not Kubernetes ordering alone), and a default-deny `NetworkPolicy` on `idp-connector` so its "no inbound Service" isolation claim actually holds against the namespace's default-allow network.

**Acceptance criteria:**
- [ ] The receiving track (whoever implements/reviews this) can act on the manifests without re-deriving the reasoning in `containerization-design.md`.
- [ ] Both images build from a pinned Go version, produce a static binary, and contain nothing but the binary (+ CA certs for `idp-connector`) — enforcement site: multi-stage Dockerfile's final `COPY` list.
- [ ] Neither Pod can write to its own root filesystem at runtime — enforcement site: `securityContext.readOnlyRootFilesystem: true`.
- [ ] `idp-connector` is unreachable except from `api-service` — enforcement site: the `NetworkPolicy` ingress rule scoped to `api-service`'s pod selector.
- [ ] A stuck `idp-connector` cannot starve `api-service` if colocated on one node — enforcement site: `resources.limits` on both Pod specs.
- [ ] The migration `Job` cannot start a second concurrent run — enforcement site: the CI pipeline's concurrency-gated deploy stage (Story 3), not a database lock.

**Depends on:** 03 — `cmd/api-service` and `cmd/idp-connector` build as real binaries (service boundaries already stable, per `PLANNING.md`); 05 — final migration tooling name for the `Job`'s invocation (already final, `05-data-ops/migration-approach.md`).

**Model/effort:** Sonnet, medium.

**Type:** Story.

**Labels:** `track-04`, `no-code-yet`.

## Story 2 — Deliver secrets to both services per 02's inventory

**Description:** Implement the Kubernetes-native `Secret` delivery mechanism in `./secrets-delivery.md`: file-mounted secrets keyed by name only in manifests, RBAC scoped per-service (and, for the JWT signing key, scoped to the issuing code path specifically, not `api-service`'s verification path), cluster-level encryption-at-rest for the `Secret` resource type, and the migration-runner credential's backend-specific provisioning (`CREATE EXTENSION pg_trgm` rights on PostgreSQL only, nothing extra on CockroachDB, no privilege model on SQLite) kept distinct from the runtime service credential (DML-only, no DDL, per 05's R4).

**Acceptance criteria:**
- [ ] 02 (the hand-off's author) confirms this implementation matches `handoff-04-secrets.md` v2's inventory without needing to re-explain any row.
- [ ] No `Secret` value appears in a manifest, image layer, build arg, or CI log — enforcement site: manifests reference names only; CI's `gitleaks` stage (Story 3).
- [ ] No ServiceAccount can `get`/`list` a `Secret` it doesn't mount — enforcement site: per-service `Role`/`RoleBinding` scoped to named `Secret`s.
- [ ] The migration-runner credential and the runtime service credential are provisioned as two distinct `Secret`s, never one — enforcement site: separate `Secret` objects, separate `Role`s, per 05's R4.
- [ ] `Secret` resources are encrypted at rest in etcd — enforcement site: cluster `EncryptionConfiguration`.

**Depends on:** 02 — `handoff-04-secrets.md` (received, v2, final); 05 — `migration-approach.md` R4 (final).

**Model/effort:** Sonnet, medium.

**Type:** Story.

**Labels:** `track-04`, `security-graded`, `no-code-yet`.

## Story 3 — CI/CD pipeline: build, lint, test, scan, migrate, describe-only deploy

**Description:** Stand up the pipeline in `./ci-pipeline.md`: build/lint/test(`-race`, three DB targets: SQLite, Postgres, CockroachDB)/security-scan/image-build on every PR; push-images/migration-job/describe-only-deploy on merge. The migration-job stage is where 05's R2 exclusivity requirement is actually enforced — a concurrency-gated pipeline stage, not a database lock, since no single lock mechanism covers both the right scope (session, to span `goose`'s per-file transactions) and the right engine (CockroachDB supports transaction-scope only).

**Acceptance criteria:**
- [ ] A developer or reviewer can act on this pipeline description without re-deriving why each stage exists.
- [ ] A lint or test failure blocks merge — enforcement site: required-status-check on the PR, not advisory.
- [ ] A CockroachDB-specific SQL incompatibility surfaces at PR time, not deploy time — enforcement site: the dedicated CockroachDB testcontainer job.
- [ ] Only one migration run can reach the migrate stage at a time, across concurrent deploys — enforcement site: the pipeline's own concurrency group/mutex on the deploy environment.
- [ ] No secret enters a build log or image layer — enforcement site: `gitleaks` stage plus the Dockerfile's minimal `COPY` list (Story 1).
- [ ] The final deploy stage is documented (which manifests, which namespace, which credential) but not invoked — enforcement site: stage exists as a named, non-executing step until a separate real-deploy decision is made.

**Depends on:** 03 — test-strategy outline for `go test ./...` to run against; 05 — `migration-approach.md` (final).

**Model/effort:** Sonnet, medium.

**Type:** Story.

**Labels:** `track-04`, `no-code-yet`.

## Story 4 — Local dev-loop: compose profiles for SQLite and Postgres

**Description:** Build the `docker-compose.yml` in `./local-dev-loop.md`: `sqlite` and `postgres` profiles matching 03's `DB_DRIVER` env var, a mock/local `idp-connector` target, hot-reload (`air`) for `api-service` in dev profiles only, and local-only dev credentials clearly distinguished from the real secrets mechanism (Story 2).

**Acceptance criteria:**
- [ ] A new developer can run either profile from this file alone, without asking what `DB_DRIVER` values are valid.
- [ ] The `postgres` profile exercises the shared `postgres`-package code path (Postgres + CockroachDB) — enforcement site: compose service definition wiring `DB_DRIVER=postgres`.
- [ ] Dev credentials never appear in `./secrets-delivery.md`'s mechanism or in any real manifest — enforcement site: explicit statement in the file plus separate variable naming (`dev`/`dev`, hardcoded client id/secret).
- [ ] Hot-reload has zero effect on the production image build — enforcement site: `air` target scoped to compose dev profiles only, never `./containerization-design.md`'s Dockerfile.

**Depends on:** 03 — `DB_DRIVER`/config-surface env vars (already stable, per `PLANNING.md`).

**Model/effort:** Sonnet, low.

**Type:** Story.

**Labels:** `track-04`, `no-code-yet`.

## Story 5 — Observability: logging, metrics, never-log enforcement, retention-sweep alerting

**Description:** Implement `./observability.md`: structured `slog` logging with an explicit-field discipline (never a whole request/response struct through a log call), a lint check flagging never-log-list field names, RED metrics with no PII-cardinality labels, ingress-level query-string redaction for search endpoints, RBAC-separated audit-log read access, and 05's S5 retention-sweep requirement — rows-examined/rows-deleted/oldest-surviving-row-age metrics per data class, with an alert when oldest-row-age exceeds retention-window-plus-sweep-interval, since a dead sweep job produces zero rows-deleted either way and gives no other symptom.

**Acceptance criteria:**
- [ ] 02 confirms the never-log list (`handoff-04-secrets.md`'s nine items) is fully covered by a named enforcement site, not just a policy statement.
- [ ] No password, token, JWT, client secret, or DSN reaches a log call — enforcement site: explicit-field logging discipline plus the CI lint check.
- [ ] Search-endpoint name/phone query fragments never appear in ingress access logs — enforcement site: ingress log-format override / redaction.
- [ ] Audit-log read access is a distinct RBAC grant from Secret-read access — enforcement site: separate `Role` bound to an incident-review group.
- [ ] A stopped retention-sweep job triggers an alert within one sweep interval of its window being exceeded — enforcement site: the `oldest_surviving_row_age_seconds` alert rule, per data class.

**Depends on:** 02 — `handoff-04-secrets.md`'s never-log list (final); 05 — `pii-governance.md` S5 retention numbers and data-class names (final).

**Model/effort:** Sonnet, medium.

**Type:** Story.

**Labels:** `track-04`, `security-graded`, `no-code-yet`.

## Story 6 — Derived-data deletion / log-retention enforcement (placeholder, pending consistency pass)

**Description:** Placeholder for the enforcement site of "logs and any derived/cached data must not outlive the data they're derived from" — the interaction the consistency pass is explicitly checking (`PLAN.md` "Retention and deletion": `05/pii-governance.md` S5 numbers and deletion-job contract vs. `04/observability.md` vs. `02/threat-model.md` assumptions). 04 is the presumptive owner of the enforcement site if the pass confirms it's a log-retention/observability-pipeline mechanism (e.g., a log-sink retention policy matching 05's window, not a separate deletion job) rather than a DAO-level or connector-level mechanism. Not filled in beyond this placeholder until the pass reports, per team-lead's instruction — this story exists so the gap is tracked now rather than discovered after transcription.

**Acceptance criteria:**
- [ ] Left open pending the consistency pass's ruling on which track owns the enforcement site.

**Depends on:** 02 — `threat-model.md` assumptions about derived-data lifetime; 05 — `pii-governance.md` S5 deletion-job contract and retention numbers.

**Model/effort:** TBD, pending scope.

**Type:** Story.

**Labels:** `track-04`, `from-consistency-pass`, `no-code-yet`.
