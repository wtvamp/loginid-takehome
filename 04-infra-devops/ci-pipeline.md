# CI/CD Pipeline Design

Design-only — a description of what would run, not a committed workflow file. Targets a generic CI platform (GitHub Actions shape, since the repo is on GitHub) but nothing here is Actions-specific beyond syntax.

## Stages, in order, on every PR

1. **Build.** `go build ./...` for both `cmd/api-service` and `cmd/idp-connector`, against the module's declared Go version. Fails fast — nothing downstream runs against code that doesn't compile.
2. **Lint.** `golangci-lint run` (staticcheck, govet, unused, errcheck at minimum). Non-negotiable gate, not advisory — a lint failure blocks merge, same as a test failure.
3. **Test.** `go test ./... -race` — unit tests against 03's `internal/dao` per-backend implementations: SQLite in-process for the fast path, plus **two** testcontainer jobs against the shared `postgres` package — one on Postgres, one on CockroachDB. Considered running CockroachDB only in the local dev-loop (wire-compatible enough with Postgres that testing both looked redundant) and reconsidered for CI specifically: wire-compatible isn't schema-compatible, and a Postgres-only test matrix means a CockroachDB-specific SQL quirk (UUID defaults, upsert syntax, lock semantics) surfaces at deploy time instead of at PR time. CockroachDB ships an official single-node Docker image, so this is a config addition to the test job, not new infrastructure — a few extra minutes of CI wall-clock for catching it at the cheapest possible point. `-race` is not optional given two DB implementations with genuine concurrent-access paths worth catching in-process.
4. **Security scan.** Two checks, both fast enough to run on every PR:
   - `govulncheck ./...` — known-CVE dependency scan.
   - `gitleaks` (or equivalent secret-pattern scanner) over the diff — a second line of defense behind "secrets never enter source" (per 02's never-log/never-commit discipline), not a replacement for it.
5. **Build images.** Both Dockerfiles (`./containerization-design.md` §2) built and tagged with the commit SHA. Not pushed yet on a PR build — build-only, to catch a broken multi-stage build before merge.

## On merge to main

6. **Push images** to the registry, tagged with SHA and `latest` (or a semver tag if the project adopts one — not decided, not needed for a take-home).
7. **Run migration job** — describe-only in this design, per `../05-data-ops/migration-approach.md`, **final**: the pipeline invokes `goose` against the migration-runner credential, gated so only one pipeline run at a time can reach this stage. Final reasoning (05's R2, corrected from an earlier draft): CockroachDB's `pg_try_advisory_lock` variants silently no-op rather than erroring, but even a working lock scope doesn't solve this — a transaction-scoped lock (the only kind CockroachDB supports) releases between `goose`'s per-file transactions, letting a second runner interleave mid-run, while a session-scoped lock that would span the run isn't available on CockroachDB at all. No database mechanism covers both the right scope and the right engine, so exclusivity lives entirely outside the database: a concurrency group / mutex on the pipeline's own deploy environment. Note `goose fix` (§1 of the migration doc) is a pre-first-application step, not a pre-tag tidy-up — irrelevant to this pipeline's own stages but worth remembering if a `make` target ever wraps it.
8. **Deploy — describe-only, not executed.** A named final stage: "apply manifests to the lab cluster via amber-kubernetes." This step is documented as what *would* run — which manifests, which namespace, which credential applies them — but is never actually invoked during this take-home, consistent with the deployment-target note in `./PLAN.md`. If a real deploy is ever wanted, this is the stage that gets un-stubbed, not redesigned.

## What's deliberately not in this pipeline

No canary/blue-green deploy strategy, no multi-environment promotion pipeline (dev → staging → prod), no automated rollback orchestration beyond "the migration job's `backoffLimit` and the Deployment's own rollout history." A take-home doesn't need a release engineering platform; it needs to show the shape of one.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from 03's test-strategy outline and the containerization design above. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
