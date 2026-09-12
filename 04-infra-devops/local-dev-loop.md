# Local Dev-Loop

Design-only — a docker-compose sketch, not a committed `docker-compose.yml`. Local counterpart to the cluster target in `./containerization-design.md`, not a replacement for it: this is what a developer runs on a laptop; the cluster design is what the submission targets as the real deployment shape.

## Two compose profiles, matching 03's `DB_DRIVER` env var

```yaml
# docker-compose.yml (sketch)
services:
  api-service:
    build: {context: ., dockerfile: cmd/api-service/Dockerfile}
    environment:
      DB_DRIVER: ${DB_DRIVER:-sqlite}
      DB_DSN_FILE: /run/secrets/db_dsn
      HTTP_ADDR: ":8080"
    ports: ["8080:8080"]
    volumes: ["sqlite-data:/data"]
    secrets: [db_dsn]
    profiles: ["sqlite"]

  api-service-pg:
    extends: {service: api-service}
    environment:
      DB_DRIVER: postgres
    depends_on: [postgres]
    profiles: ["postgres"]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: dev
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
    ports: ["5432:5432"]
    secrets: [postgres_password]
    profiles: ["postgres"]

  idp-connector:
    build: {context: ., dockerfile: cmd/idp-connector/Dockerfile}
    environment:
      IDP_ABC_BASE_URL: ${IDP_ABC_BASE_URL:-http://mock-idp:9090}
    secrets: [idp_abc_client_id, idp_abc_client_secret]
    profiles: ["sqlite", "postgres"]

secrets:
  db_dsn: {file: ./.dev-secrets/db_dsn}                       # untracked; developer creates once, e.g. "file:/data/dev.db" or a postgres:// DSN
  postgres_password: {file: ./.dev-secrets/postgres_password}
  idp_abc_client_id: {file: ./.dev-secrets/idp_abc_client_id}
  idp_abc_client_secret: {file: ./.dev-secrets/idp_abc_client_secret}

volumes:
  sqlite-data:
```

`.dev-secrets/` is `.gitignore`d; a `.dev-secrets/README` (or a `make dev-secrets-init` target) writes the four files with placeholder dev values on first run. This isn't cosmetic — the inventory's rules apply to every environment, including dev: row 5 says vendor client credentials are "never in source or committed config", and row 1's `DB_DSN_FILE` is "Required, not preferred," with no development exemption written anywhere. Compose's file-based `secrets:` mechanism is the local equivalent of the `Secret` volume mounts in `./secrets-delivery.md` — same shape (a file path the process reads), just backed by a bind-mounted file instead of a Kubernetes object.

`docker compose --profile sqlite up` for the zero-dependency path (no Postgres container, fastest inner loop); `docker compose --profile postgres up` when a developer needs to exercise the Postgres-specific code path (the `pg_trgm` index, per `../05-data-ops/multi-db-strategy.md`). CockroachDB is omitted here for loop speed, not because it's redundant — the CockroachDB code path (`withRetry`'s engine-aware retry, `multi-db-strategy.md`) is genuinely different code from the Postgres path, which is exactly why `./ci-pipeline.md` runs a dedicated CockroachDB job; the local loop just doesn't need every backend on every save.

## Dev credentials are dev credentials

The values developers put in `.dev-secrets/*` are local-only, never used outside a developer's own machine, and never the pattern used in `./containerization-design.md` or `./secrets-delivery.md` — those go through the mechanisms 02 specifies. Worth stating explicitly so nobody reads this file as the security design.

## Migrations locally

`goose up` (or whatever 05 finalizes) run by hand or via a `make migrate` target against whichever profile is active — no orchestration-layer exclusivity needed locally since there's exactly one developer and one instance. The R2 requirement (`../05-data-ops/migration-approach.md`) is a production/CI concern; the local loop doesn't need to simulate it.

## Hot-reload for `api-service`, dev-profile only

Considered cutting `air` as unneeded polish, reconsidered: `docker compose build && up` on every change is a slow inner loop for a project a developer iterates on repeatedly, and the cost of adding it is close to zero — one `air`-based dev target in the compose file, no production image impact since it only applies to the `sqlite`/`postgres` dev profiles, never `./containerization-design.md`'s images. Worth doing, not worth debating further.

## What's deliberately not here

No local Kubernetes (`kind`/`minikube`) mirroring the lab cluster — compose is the right altitude for a two-service Go project's local loop; a local k8s cluster would be solving a problem this project doesn't have.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from 03's config surface and the containerization design above. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
