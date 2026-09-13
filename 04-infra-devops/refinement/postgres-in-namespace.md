# New story refinement — deploy real Postgres in `loginid-takehome`, wire migrations, DB_DSN_FILE

**Product Owner:** Naomi Voss (01). **Research team:** Imogen Hale, Desmond Okafor, Tobias Lindqvist. **Engineering:** Theo Bergman (04, owner); Priya Nandakumar (05, reviewer). **Source:** gap found during LT-40 refinement — none of `04-infra-devops/backlog.md`'s existing stories (LT-45/46/47/48/49/LT-20) actually deploy a running database instance; they all assume a DB target already exists. **Jira key: LT-52** ("Database provisioning in-namespace: Postgres, DB credentials Secret, migration Job, DB_DSN_FILE wiring"), under Epic LT-4, linked as blocking LT-39's live review.

**Who did the work:** Refined by Naomi Voss (PO); claims check Desmond Okafor; objection Tobias Lindqvist; engineering owner Theo Bergman; reviewer Priya Nandakumar.

## Why this one, now

LT-39's handlers are code-review-passed but there's nothing behind the DAO in the live cluster — no database, no applied schema, no seed data. This is the last missing piece before LT-34/LT-39/LT-40/LT-51's joint live-URL review can exercise a real search/retrieve call against real (if empty) data, not just an auth-and-routing check.

## What this story is

A single-replica PostgreSQL `StatefulSet` in the `loginid-takehome` namespace (Theo's call, confirmed: a managed instance would be disproportionate infra/cost for this take-home's scale; a single `StatefulSet` + `PersistentVolumeClaim` is the standard minimal shape and matches this project's existing "small, isolated, no HA beyond what's needed" pattern), a DB credential `Secret` provisioned out-of-band (same discipline as LT-32's JWT signing key — never through the CI-applied manifest set), a migration `Job` that runs goose against it in the correct two-invocation order before `api-service`'s rollout, and `DB_DSN_FILE` wired into `api-service`'s config so it actually connects to this database instead of erroring on an unset DSN.

**Explicit scope limit, stated up front:** CockroachDB is exercised in CI only (LT-47's testcontainer job, against the shared `postgres`-package code path) and is **not** deployed anywhere in this cluster. Only PostgreSQL runs live. This is a deliberate scope limit for a take-home demo, not an oversight — stated here so a reviewer doesn't reasonably expect to find CockroachDB running somewhere and conclude something's missing.

## Acceptance criteria

- [ ] A single-replica PostgreSQL `StatefulSet` (with a `PersistentVolumeClaim` for data persistence across pod restarts) runs in `loginid-takehome`. Enforcement site: the `StatefulSet` manifest and its `Service`.
- [ ] **NetworkPolicy-scoped reachability, matching the existing `idp-connector-default-deny` pattern:** Postgres is reachable only from `api-service` (and, if the migration `Job`'s pod uses a distinct label, from that pod specifically) — not from every pod in the namespace by default. Enforcement site: a default-deny `NetworkPolicy` on Postgres's pod selector, ingress scoped to `api-service`'s (and the migration `Job`'s) pod selector, matching `idp-connector`'s existing policy shape.
- [ ] The DB credential (connection string / password) is provisioned out-of-band, never through the CI-applied manifest set — matching the pattern `deploy/issuer-secret-bootstrap.yaml` already established for the JWT signing key. Enforcement site: the credential `Secret` is absent from `deploy/manifests.yaml`; a bootstrap doc/script analogous to `issuer-secret-bootstrap.yaml` documents how it's created.
- [ ] Per `05-data-ops/migration-approach.md` R4, the migration-runner credential (DDL rights, plus `CREATE EXTENSION pg_trgm` on Postgres specifically) and the runtime service credential (DML-only, **cannot run DDL**) are two distinct `Secret`s, never one — matching LT-46's own criterion for this same split, applied here against a database that now actually exists. Enforcement site: the runtime credential's Postgres role has no `CREATE`/`ALTER`/`DROP` grants — a test or manual check attempting a DDL statement with the runtime credential fails with a permissions error.
- [ ] The migration `Job` runs goose in the **correct, corrected order**: the backend directory (`postgres/`, containing the actual `CREATE TABLE` DDL) **first**, then `shared/` (seed data only, currently just the `auth_method` "password" row) **second** — per `migration-approach.md`'s own corrected ordering (the original text had this inverted, caught reading the seed and DDL together, not caught by a green test suite that happened to apply files in a different working order). Enforcement site: the `Job`'s invocation script/command, explicitly two `goose` invocations in this order, each against its own version table (`goose_db_version_postgres`, `goose_db_version_shared`) per R4's two-independent-sequences requirement.
- [ ] Both goose invocations must succeed for the migration `Job` to count as successful — a partial success (backend DDL applied, `shared/` seed failed, or vice versa) is a failed `Job`, not a partial pass. Enforcement site: the `Job`'s script exits non-zero if either invocation fails.
- [ ] **The migration `Job` is idempotent** — re-running it against an already-migrated database (e.g. a redeploy) is a no-op, not an error and not a duplicate application. This is goose's own native behavior (it tracks applied versions and skips them) plus the seed migration's `ON CONFLICT (name) DO NOTHING` (per `migration-approach.md`) — enforcement site: a test or manual re-run of the `Job` against an already-migrated database, confirming it exits successfully having applied zero new versions.
- [ ] The migration `Job` runs to completion **before** `api-service`'s rollout — enforcement site: deploy pipeline ordering (readiness gate or explicit sequencing in the merge-to-main workflow), not an assumption that Kubernetes scheduling happens to land in the right order.
- [ ] `DB_DSN_FILE` (already a config surface `internal/config` supports per `PLANNING.md`'s Service boundaries) is wired to point at the runtime service credential's mounted file, **and its precedence over `DB_DSN` is exercised in the actual running pod**, not only in `internal/config`'s existing unit test — `api-service` connects successfully on startup using the file-mounted DSN. Enforcement site: `api-service`'s Deployment manifest sets `DB_DSN_FILE` (not `DB_DSN`) pointing at the mounted runtime credential; confirmed live, not just asserted by the existing unit test.
- [ ] **The health endpoint reflects real DB connectivity, without leaking the DSN or any driver `DETAIL` text.** `/healthz`'s existing `{"status","service","build"}` shape (per 02's ruling on LT-34) gains a DB-connectivity signal consistent with that ruling's own allowance for a per-dependency boolean (e.g. `"db":"ok"`/`"db":"down"`) — never a connection string, host, port, or raw driver error. Enforcement site: the health handler's DB check queries the connection (e.g. a trivial `SELECT 1`) and reports only a boolean/enum, with a test asserting the response never contains the DSN substring or any driver error text.
- [ ] `deploy/manifests.yaml`'s header comment (which currently states "no migration Job, no DB_DSN secret — those wire up config the current code doesn't use yet") is updated to reflect that this is no longer true, so a future reader isn't misled by a stale comment.
- [ ] **`ResourceQuota` headroom confirmed for the new StatefulSet pod before merge** — added per Theo's own flag, given the earlier `api-service-issuer` `FailedCreate` incident from under-sized quota. Enforcement site: the quota math is shown in the PR description (current `limits.cpu`/`limits.memory` vs. the new total including Postgres's request/limit), and the `ResourceQuota` object is adjusted in the same PR if needed — not discovered as a second `FailedCreate` after merge.

## Non-goals

- No CockroachDB deployment — CI-only, per the explicit scope limit above.
- No backup/restore, replication, or HA for Postgres — single replica, single PVC, take-home scale; `migration-approach.md` explicitly leaves backup schedule/retention as 04's call and out of this document's scope, and this story doesn't add it either.
- No connection pooling (e.g., PgBouncer) — direct connections from `api-service` are sufficient at this scale; flag if load testing ever suggests otherwise.
- No change to the migration file contents themselves (DDL, seed data) — that's 05's `migration-approach.md` and the actual `.sql` files, already written; this story only runs them.

## PO review script

1. Confirm the `StatefulSet` and its `PersistentVolumeClaim` exist in the manifest, targeting `loginid-takehome`.
2. Confirm a default-deny `NetworkPolicy` scopes Postgres's ingress to `api-service`/the migration `Job` only, matching `idp-connector-default-deny`'s existing shape.
3. Confirm the DB credential `Secret` is absent from `deploy/manifests.yaml` and documented as provisioned out-of-band, matching the JWT-signing-key pattern.
4. Confirm two distinct `Secret`s exist for the migration-runner vs. runtime credentials; confirm the runtime credential's Postgres role genuinely cannot run DDL (attempt one, expect a permissions failure).
5. Read the migration `Job`'s script directly: confirm two `goose` invocations, backend directory first, `shared/` second, each with its own `-table` flag, that the script fails if either invocation fails, and that re-running it against an already-migrated database is a clean no-op.
6. Confirm the deploy pipeline sequences the migration `Job` to completion before `api-service`'s rollout — not merely "applied at some point."
7. Confirm `DB_DSN_FILE` (not `DB_DSN`) is what's actually set in `api-service`'s live Deployment manifest — precedence exercised in the running pod, not only in the existing unit test.
8. Confirm the health endpoint's DB-connectivity field is a boolean/enum only — hit it once with a deliberately wrong DSN (or simulate DB-down) and confirm the response never contains a connection string, host, port, or driver error text.
9. Once deployed: confirm `api-service` actually connects, and run a real `profile:search`/`profile:read:own` call through the live URL (joint with LT-40/LT-51/LT-39's review) confirming it reaches real Postgres and returns a real (even if empty) result set — this is the actual proof this story exists to provide.
10. Confirm the quota math was shown and the `ResourceQuota` adjusted if needed, before merge — not discovered as a `FailedCreate` after.

## Research team review

**Desmond Okafor — claims check:** Verified the goose two-invocation ordering claim near-verbatim against `migration-approach.md`'s "Mechanism 04 needs" section, including the "corrected order" framing (not an overstatement — the source's own correction note matches the doc's compression of it faithfully). Verified the R4 credential-split claim exactly, including the Postgres-specific `pg_trgm` carve-out. Verified "matching LT-46's own criterion" is not a loose analogy — checked `04-infra-devops/backlog.md` directly, LT-46 states the identical requirement verbatim. No corrections needed.

**Tobias Lindqvist — objection:** Raised that Postgres — the first component in this namespace actually holding PII at rest — had no stated `NetworkPolicy`, the same class of miss as the JWT-signing-key RBAC gap from LT-32. Objection was based on an earlier copy of this doc, sent before the Jira ticket's NetworkPolicy criterion was incorporated — the criterion (default-deny scoped to `api-service` + the migration `Job`'s pod selector, matching `idp-connector`'s pattern) was already present by the time he reviewed. Confirmed to him directly. His broader process point stands independently: this is the second time a new stateful/secret-bearing component needed a research-team catch on network/access scoping — flagged to Theo as a candidate standing checklist item, not decided unilaterally here.

**Naomi Voss — ruling:** This refinement is accepted and ready to hand off. Both reviewers' passes came back clean on the doc as it stands; Tobias's process observation (recurring category of miss across two stories) is valuable independent of this instance already being covered, and has been routed to Theo for a process decision rather than folded into this story's own criteria.

## PO acceptance review — PR #29

**Reviewer:** Naomi Voss (PO). **Date:** 2026-09-13. **PR:** `https://github.com/wtvamp/loginid-takehome/pull/29`.

**Blocking defect found and confirmed empirically, not from reading alone.** Postgres 15+ revoked `PUBLIC`'s implicit `CREATE` grant on the `public` schema — only the schema owner and superusers get it by default. The `01-create-roles.sh` initdb script grants `migrator` `CREATEDB` and "ALL PRIVILEGES ON DATABASE," but never grants schema-level `CREATE` — a database-level grant and a schema-level grant are different things in Postgres. I ran a real `postgres:16-alpine` container, replayed the script's exact `CREATE ROLE`/`GRANT`/`ALTER DEFAULT PRIVILEGES` sequence, then attempted `CREATE TABLE t (id int);` as `migrator`: **`ERROR: permission denied for schema public`**. Confirmed the fix in the same container — one line, `GRANT CREATE ON SCHEMA public TO migrator;` — after which the same `CREATE TABLE` succeeded. Also confirmed `CREATE EXTENSION IF NOT EXISTS pg_trgm` (the first migration's other DDL statement) succeeds with the same single fix, since `pg_trgm` is a "trusted" extension (installable by any role with schema-level `CREATE`, not just a superuser) as of Postgres 13. This would have failed the very first real deploy, and `kubectl apply --dry-run=server` cannot catch it — it validates Kubernetes objects, not the SQL inside a `ConfigMap`.

**Everything else checked out clean, verified directly against the diff, not summarized from the PR description:**
- `StatefulSet` + `PersistentVolumeClaim` + headless `Service`: present, correctly shaped.
- Default-deny `NetworkPolicy` on Postgres, ingress scoped to exactly `api-service` and the migration `Job`'s pod label — matches `idp-connector`'s pattern exactly, and a bonus: a matching `NetworkPolicy` was also added for `api-service-issuer` (in-cluster JWKS callers + `ingress-nginx` for the public `/auth/token` route), proactively applying the new §5a checklist beyond this story's own scope.
- Three `Secret`s provisioned out-of-band (`db-secret-bootstrap.yaml`) — confirmed absent from `deploy/manifests.yaml`; `deploy/rbac.yaml`'s `Role` still grants nothing on `secrets`.
- Migration-runner vs. runtime credential split confirmed as two distinct `Secret`s with distinct roles; runtime's lack of DDL rights is real (once the above fix lands) — `ALTER DEFAULT PRIVILEGES` correctly grants DML-only to `runtime` on tables `migrator` creates.
- goose invocation order confirmed correct against the actual `migrations/` directory structure (`postgres/` then `shared/`, matching the real file layout, not just the doc's claim), each with its own `-table` flag; both must succeed (`set -e` in the Job's shell script).
- Idempotency: goose's own applied-version tracking plus the seed's `ON CONFLICT DO NOTHING` — by construction, matches the criterion.
- `api-service` correctly split into its own manifest and sequenced after the migration `Job` completes (`kubectl wait --for=condition=complete`), not assumed ordering.
- Quota math shown in the manifest's own comment, itemized per component, matching the PR description exactly.
- `deploy/manifests.yaml`'s header comment updated to reflect LT-52, no longer stale.
- Health-endpoint DB-connectivity field confirmed as a separate, paired PR from Renata (03) — correctly not claimed as done here.

**Verdict: blocked on one confirmed defect, otherwise ready.** Reported to Theo with the exact fix and the empirical verification. Re-reviewing once pushed; everything else above stands and won't need re-checking.

## PO acceptance review — PR #29, commit `39b49bc`

**Reviewer:** Naomi Voss (PO). **Date:** 2026-09-13.

**My schema-CREATE fix confirmed applied and correctly commented** (`GRANT CREATE ON SCHEMA public TO migrator`, with the reasoning and the "verified by 01/Naomi" attribution inline) — Theo independently re-verified it himself in a fresh container before trusting my report, which is exactly the right discipline.

**Additional changes since my last pass, each independently re-verified rather than taken on report:**
- **`postgres:16-alpine` runs as UID 70, not root or an assumed 999.** Confirmed myself: `docker run --rm --entrypoint getent postgres:16-alpine passwd postgres` → `postgres:x:70:70:...`. The `runAsNonRoot: true, runAsUser: 70, runAsGroup: 70, fsGroup: 70` fix is correct and matches the same class of issue LT-45's distroless images already had to solve.
- **`goose` genuinely supports `GOOSE_DRIVER`/`GOOSE_DBSTRING`/`GOOSE_MIGRATION_DIR` as environment variables**, not just CLI args — confirmed by building the actual pinned version (`goose@v3.22.1`) in a real container and reading its own `--help` output, which documents exactly this env-var form. The DSN (containing a password) genuinely never appears in `ps` output inside the pod with this change — a real improvement over the prior positional-argument form, matching the discipline `DB_DSN_FILE` already uses for `api-service`.
- **The sequences/UUID caveat comment is accurate and appropriately scoped** — the schema is UUID-keyed throughout (confirmed against `multi-db-strategy.md` in an earlier pass), so `ALTER DEFAULT PRIVILEGES ... ON TABLES` genuinely has no live sequence gap today; the comment correctly states this as a latent (not active) risk for a future `SERIAL`/`IDENTITY` column, not a defect needing a fix now.
- **The `kubectl wait` → explicit poll-loop replacement is a real, well-reasoned fix**, not just defensive rewriting: `kubectl wait --for=condition=complete` doesn't fail fast on a `Failed` Job — it waits out the full timeout regardless, then exits as a timeout rather than a detected failure. The safety property (blocking `api-service`'s apply) held either way, but the poll loop makes a genuine migration failure surface in seconds instead of the full 180s timeout, and dumps the Job's logs on failure. The word-boundary match (`case " $conditions " in *" Complete "*)`) is the correct way to test membership in Kubernetes' space-separated multi-condition string — a plain `=` comparison, as the inline comment notes, would never have matched.
- **TLS on the Postgres connection (`sslmode=require`, a self-signed `cert-manager` `Issuer`/`Certificate`)** — this is Marcus's ruling (`threat-model.md` Assumption 9), consistently applied with the client-facing TLS requirement one hop earlier. I did not independently stand up cert-manager to re-verify the handshake myself (a heavier repro than the container tests above), so this one line item rests on Theo's own reported end-to-end scratch-namespace test (TLS on, migrator creates tables, runtime blocked from DDL, both goose invocations succeed) rather than my own empirical confirmation — flagged honestly rather than claimed as independently verified.

**Verdict: PASSES.** The one blocking defect from my prior pass is fixed and independently re-confirmed; every other change since then is either independently verified by me directly (UID, goose env vars, poll-loop logic, sequences caveat) or, for the one item I couldn't cheaply re-verify myself (the TLS handshake), honestly attributed to Theo's own reported test rather than claimed as mine. Posting this to the PR; merge can proceed once Priya's post lands too.

## PO acceptance review — live deployment (all criteria but #5)

**Reviewer:** Naomi Voss (PO). **Date:** 2026-09-13. Prepared now, ahead of criterion 5 landing, per team-lead's request, so acceptance is a single pass once it does.

**Directly, independently confirmed by me against the live URL** (`curl --resolve loginid-takehome.uplifttech.org:443:67.168.194.236 https://loginid-takehome.uplifttech.org/healthz`): HTTP 200, `{"status":"ok","service":"api-service","build":"aead77e3d0e3d1fe439dd4109ef10878377e1ab3"}` — the service is up and serving a real, current commit SHA.

**Reported by Theo/team-lead, not independently re-run by me against the live cluster (I have no cluster access) — recorded as attributed evidence, consistent with this review's own discipline of not overclaiming what I actually checked:**
- Migrations applied (versions 2 + 1, matching the two-directory split); the `runtime` role's DDL denial confirmed live (`CREATE TABLE` → permission denied; `SELECT` works) — this is the live-cluster instance of the same permission model I verified locally in a scratch container during the PR #29 review, now confirmed against the real deployed database.
- The second deploy's migration `Job` re-run reported "no migrations to run" for both directories — satisfies the idempotency criterion.
- All Deployments Ready.

**Criteria status:**
- [x] StatefulSet/PVC, NetworkPolicy scoping, out-of-band Secret provisioning, migrator/runtime split, goose ordering, idempotency, pre-rollout sequencing, quota headroom, header-comment accuracy — all confirmed via PR #29's code review (above) plus this live-deployment evidence.
- [x] **Criterion 5 — closed.** PR #33 adds `db: "ok"|"down"` to `/healthz` in verifier-mode `api-service`, via a separate, bounded (1s) `PingContext` check independent of `dao.Repository` (05's contract exposes no `Ping`, correctly avoided amending it for a one-field health signal). Confirmed live at the public URL: `{"status":"ok","service":"api-service","build":"c1fb69d...","db":"ok"}` — only these four fields, no DSN/host/port. Pulled the PR and ran the tests myself rather than trusting the description: `go build`/`go test ./internal/app/...` all pass, including `TestHealthzWithDB_Unreachable_ReportsDownNotError`, which asserts the response body never contains the DSN substring or driver error text when the DB is genuinely unreachable — the one path I can't exercise against the live cluster myself, confirmed by this dedicated test instead. `/healthz` correctly stays 200/`"status":"ok"` even when `db` is `"down"` (a DB outage fails the DB-backed routes, not the process's own liveness probe) — consistent with LT-40's fail-closed design.

## PO acceptance review — LT-52 closing verdict

**Reviewer:** Naomi Voss (PO). **Date:** 2026-09-13.

All five criteria now confirmed: PVC/TLS (PR #29, independently verified — UID, goose env vars, schema-CREATE fix, poll-loop logic), migrations + idempotent re-run (live-deployment evidence, versions 2+1, second-deploy no-op), runtime role DDL-denied (verified locally in a scratch container during PR #29's review, reproduced live per team-lead's report), `DB_DSN_FILE` wired and exercised in the running pod, and now the health boolean (PR #33, tests run myself).

**Verdict: PASS. LT-52 is Done.** Third story closed end to end — refined, implemented, reviewed (including two independently-verified defects along the way: the schema-CREATE bug in PR #29, caught before merge; nothing outstanding now), and confirmed live at the public URL.
