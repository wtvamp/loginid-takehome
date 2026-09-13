# Track 04 — Infra & DevOps Backlog

Track: 04-infra-devops. Lead: Theo Bergman. Epic: "Infra & DevOps: Containerization, CI/CD, and Operations" (key: LT-4).

Authored now per Phase 4; will be re-checked against the consistency pass's cross-cutting findings (`PLAN.md` §"Cross-cutting — enforcement sites") once that report lands, particularly for Story 6 below. All stories carry `no-code-yet` — Warren's Phase 4 go covers backlog authoring, not implementation; a separate, explicit code go-ahead is still required before any `.go`, Dockerfile, CI config, or compose file is written for real.

## Story 1 — Containerize `api-service` and `idp-connector` (LT-45)

**Description:** Build the two multi-stage Go images (distroless final stage, non-root, read-only root filesystem) and the Deployment/Service/NetworkPolicy manifests targeting the lab cluster, per `./containerization-design.md`. Covers both binaries' Dockerfiles, resource requests/limits sized per service (CPU-bound `api-service` vs. I/O-bound `idp-connector`), the migration `Job` (parallelism 1, gated by the CI pipeline's exclusivity mechanism, not Kubernetes ordering alone), a default-deny `NetworkPolicy` on `idp-connector` so its "no inbound Service" isolation claim actually holds against the namespace's default-allow network, and — per the consistency pass's F13 finding — a **third manifest**: `api-service-issuer`, a second Deployment of the same `api-service` image (`APP_MODE=issuer`) whose own ServiceAccount is the only one that mounts the JWT signing key, since RBAC on the `Secret` verb does nothing once it's volume-mounted into the verifying Deployment's replicas.

**Acceptance criteria:**
- [ ] The receiving track (whoever implements/reviews this) can act on the manifests without re-deriving the reasoning in `containerization-design.md`.
- [ ] Both images build from a pinned Go version, produce a static binary, and contain nothing but the binary (+ CA certs for `idp-connector`) — enforcement site: multi-stage Dockerfile's final `COPY` list.
- [ ] Neither Pod can write to its own root filesystem at runtime — enforcement site: `securityContext.readOnlyRootFilesystem: true`.
- [ ] `idp-connector` is unreachable except from `api-service` — enforcement site: the `NetworkPolicy` ingress rule scoped to `api-service`'s pod selector.
- [ ] A stuck `idp-connector` cannot starve `api-service` if colocated on one node — enforcement site: `resources.limits` on both Pod specs.
- [ ] The migration `Job` cannot start a second concurrent run — enforcement site: the CI pipeline's concurrency-gated deploy stage (Story 3), not a database lock.
- [ ] The JWT signing private key is mounted into `api-service-issuer` only, never into `api-service`'s own Deployment — enforcement site: `api-service`'s manifest carries no volume/mount referencing that `Secret` at all, verified by manifest inspection, not RBAC.

**Depends on:** 03 — `cmd/api-service` and `cmd/idp-connector` build as real binaries (service boundaries already stable, per `PLANNING.md`); 05 — final migration tooling name for the `Job`'s invocation (already final, `05-data-ops/migration-approach.md`).

**Model/effort:** Sonnet, medium.

**Type:** Story.

**Labels:** `track-04`, `no-code-yet`.

## Story 2 — Deliver secrets to both services per 02's inventory (LT-46)

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

## Story 3 — CI/CD pipeline: build, lint, test, scan, migrate, describe-only deploy (LT-47)

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

## Story 4 — Local dev-loop: compose profiles for SQLite and Postgres (LT-48)

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

## Story 5 — Observability: logging, metrics, never-log enforcement, retention-sweep alerting (LT-49)

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

## Story 6 — Configure log-sink retention for the audit trail (LT-20)

**Description:** The consistency pass (`../decisions/cross-track-consistency.md`, "Deletion gap") ruled the enforcement site: a subject-deletion request reaches `user_profile`/`user_credential` by cascade but not application logs, metrics, or the audit trail, none of which has a `source` column or a deletion hook. 02's never-log list is the upstream control (PII values never enter a log in the first place, per `./observability.md`'s enforcement mechanism), which leaves exactly one remaining artifact needing a retention window: the audit trail itself (`sub`, scope, opaque record id, timestamp) — pseudonymous, not PII-free, and after a profile deletion the only remaining record that a given record id was looked up by a given caller. The enforcement site for its expiry is a retention policy on the log sink 04 already operates (Loki or equivalent, per `./observability.md`'s "Audit-log access" section) — not a deletion job, not a DAO method. 05 adds one row to `pii-governance.md`'s retention table for the audit-log window (basis: unknown, pending legal); 02 adds the two vocabulary/list fixes the pass identified for the never-log list (driver-error detail, `username`). This story configures the sink's retention setting to that window once it's a number.

**Acceptance criteria:**
- [ ] The hand-off standard: 02 and 05 can each confirm their half (never-log list scope; retention-table row) without needing this story rewritten.
- [ ] The audit-log stream expires no later than 05's stated window — enforcement site: the log sink's retention configuration (a Loki `retention_period` or equivalent for the chosen sink), not application code.
- [ ] The retention configuration is separate from, and does not touch, any other log stream's retention or any Secret-access RBAC (`./secrets-delivery.md`) — enforcement site: sink-level retention is scoped per log stream/index, not global.
- [ ] Confirmed this is a configuration value, not a deletion job: no new DAO method, no new `cmd/` entrypoint, no scheduled job beyond what `./observability.md`'s retention-sweep story (Story 5) already covers for `user_profile`/`user_credential`-derived data.

**Depends on:** 02 — never-log list additions (driver-error detail, `username`) confirmed; 05 — `pii-governance.md` audit-log retention-window row (basis: unknown, pending legal, per the pass).

**Model/effort:** Sonnet, low.

**Type:** Story.

**Labels:** `track-04`, `from-consistency-pass`, `no-code-yet`.

---

## Story 7 — Database provisioning in-namespace: Postgres, DB credentials Secret, migration Job, DB_DSN_FILE wiring (LT-52)

**Status: new** — created from a gap found during 01's LT-40 refinement (`03-engineering-delivery/refinement/LT-40.md`): none of this track's existing stories (LT-45/46/47/48/49/LT-20) actually deploy a running database instance into `loginid-takehome` — they all assume a DB target already exists. Without it, LT-39's handlers have nothing real behind the DAO, and LT-34/LT-39/LT-40/LT-51's joint live-URL review can't exercise a genuine data path. Full refinement: `04-infra-devops/refinement/postgres-in-namespace.md`.

**Description:** A single-replica PostgreSQL `StatefulSet` (with a `PersistentVolumeClaim`) in `loginid-takehome` — a managed instance would be disproportionate infra/cost for this take-home's scale, and a single `StatefulSet` matches this project's existing "small, isolated, no HA beyond what's needed" pattern (Theo's call, confirmed). Plus: a DB credential `Secret` provisioned out-of-band per the pattern already established for the JWT signing key (`deploy/issuer-secret-bootstrap.yaml`); the two-invocation goose migration `Job` in the corrected order (backend directory first, then `shared/`, per `05-data-ops/migration-approach.md`'s own corrected ordering); and `DB_DSN_FILE` wiring so `api-service` actually connects. **CockroachDB is explicitly out of scope for deployment** — it's exercised in CI only (LT-47's testcontainer job), never deployed live in this cluster; stated here so a reviewer doesn't expect to find it running.

**Acceptance criteria:**
- [ ] Single-replica PostgreSQL `StatefulSet` + `PersistentVolumeClaim` running in `loginid-takehome`.
- [ ] A default-deny `NetworkPolicy` scopes Postgres's ingress to `api-service` (and the migration `Job`'s pod) only — matching `idp-connector-default-deny`'s existing shape, not the namespace's default-allow.
- [ ] DB credential `Secret` provisioned out-of-band, never through the CI-applied manifest set — same discipline as the JWT signing key.
- [ ] Migration-runner credential (DDL rights) and runtime service credential (DML-only, cannot run DDL) are two distinct `Secret`s — enforcement site: the runtime role's Postgres grants exclude `CREATE`/`ALTER`/`DROP`, verified by a failed DDL attempt under that role.
- [ ] Migration `Job` runs goose in the corrected order — backend directory (`postgres/`) first, `shared/` second — each against its own version table (`goose_db_version_postgres`, `goose_db_version_shared`); both invocations must succeed for the `Job` to count as successful.
- [ ] The migration `Job` is idempotent — a re-run against an already-migrated database applies zero new versions and exits successfully, not an error.
- [ ] The migration `Job` completes before `api-service`'s rollout — deploy pipeline ordering, not assumed Kubernetes scheduling.
- [ ] `DB_DSN_FILE` (not `DB_DSN`) is set in `api-service`'s live Deployment, pointing at the mounted runtime credential — precedence exercised in the running pod, not only in `internal/config`'s existing unit test.
- [ ] `/healthz` reflects real DB connectivity as a boolean/enum field only (consistent with 02's existing ruling on the endpoint's response shape) — never a connection string, host, port, or driver `DETAIL` text, even when the DB is unreachable.
- [ ] `deploy/manifests.yaml`'s header comment (currently: "no migration Job, no DB_DSN secret") is updated to match reality.
- [ ] `ResourceQuota` headroom for the new `StatefulSet` pod confirmed and adjusted in the same PR if needed — shown in the PR description, not discovered as a second `FailedCreate` (per the `api-service-issuer` incident).

**Depends on:** 03 — `api-service`'s `DB_DSN_FILE` config surface (already stable); 05 — `migration-approach.md` (final, corrected ordering).
**Blocks:** LT-39's live-URL review (joint with LT-40/LT-51).
**Model/effort:** Sonnet, medium.
**Type:** Story.
**Labels:** `track-04`, `security-graded`.

---

## Story 8 — Small hardening: wait for Postgres Service DNS before the migration Job connects (follow-up to LT-52)

**Status: new** — not blocking, flagged by team-lead after the first real deploy. The migration `Job`'s first attempt hit `lookup postgres.loginid-takehome.svc ... no such host` — Postgres's `StatefulSet` rollout reported Ready before its headless `Service`'s DNS record had actually propagated. The `Job`'s own `backoffLimit: 2` retry absorbed it cleanly (second attempt succeeded), so this didn't block the deploy — but it's a race worth closing rather than relying on retry luck indefinitely.

**Description:** Add an `initContainer` (or a small wait loop in the main container's command) to the migration `Job` that polls DNS resolution of `postgres.loginid-takehome.svc` (or a TCP connect to `postgres:5432`) before invoking `goose`, so the first real attempt doesn't depend on winning a race against Service registration.

**Acceptance criteria:**
- [ ] Migration `Job` waits for Postgres to be reachable (DNS + TCP) before running goose, not just for the `StatefulSet`'s own readiness probe.
- [ ] No change to `backoffLimit` or the goose invocation order/logic — this is additive, not a replacement for existing retry behavior.

**Depends on:** none — small, self-contained follow-up to LT-52.
**Model/effort:** Sonnet, low.
**Type:** Task.
**Labels:** `track-04`.
