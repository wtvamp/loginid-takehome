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
      DB_DSN: ${DB_DSN:-file:/data/dev.db}
      HTTP_ADDR: ":8080"
    ports: ["8080:8080"]
    volumes: ["sqlite-data:/data"]
    profiles: ["sqlite"]

  api-service-pg:
    extends: {service: api-service}
    environment:
      DB_DRIVER: postgres
      DB_DSN: postgres://dev:dev@postgres:5432/loginid_dev?sslmode=disable
    depends_on: [postgres]
    profiles: ["postgres"]

  postgres:
    image: postgres:16-alpine
    environment: {POSTGRES_USER: dev, POSTGRES_PASSWORD: dev, POSTGRES_DB: loginid_dev}
    ports: ["5432:5432"]
    profiles: ["postgres"]

  idp-connector:
    build: {context: ., dockerfile: cmd/idp-connector/Dockerfile}
    environment:
      IDP_ABC_BASE_URL: ${IDP_ABC_BASE_URL:-http://mock-idp:9090}
      IDP_ABC_CLIENT_ID: dev-client
      IDP_ABC_CLIENT_SECRET: dev-secret
    profiles: ["sqlite", "postgres"]

volumes:
  sqlite-data:
```

`docker compose --profile sqlite up` for the zero-dependency path (no Postgres container, fastest inner loop); `docker compose --profile postgres up` when a developer needs to exercise the Postgres-specific code path (the `pg_trgm` index, per `../05-data-ops/multi-db-strategy.md`). CockroachDB isn't in the local loop — it's wire-compatible enough with Postgres per 05's strategy doc that the Postgres profile exercises the shared code path; a CockroachDB-specific compose profile would test the same code again, not new code.

## Dev credentials are dev credentials

The `dev`/`dev` Postgres password and the hardcoded `dev-client`/`dev-secret` above are local-only, never used outside a developer's own machine, and never the pattern used in `./containerization-design.md` or `./secrets-delivery.md` — those go through the mechanisms 02 specifies. Worth stating explicitly so nobody reads this file as the security design.

## Migrations locally

`goose up` (or whatever 05 finalizes) run by hand or via a `make migrate` target against whichever profile is active — no orchestration-layer exclusivity needed locally since there's exactly one developer and one instance. The R2 requirement (`../05-data-ops/migration-approach.md`) is a production/CI concern; the local loop doesn't need to simulate it.

## Hot-reload for `api-service`, dev-profile only

Considered cutting `air` as unneeded polish, reconsidered: `docker compose build && up` on every change is a slow inner loop for a project a developer iterates on repeatedly, and the cost of adding it is close to zero — one `air`-based dev target in the compose file, no production image impact since it only applies to the `sqlite`/`postgres` dev profiles, never `./containerization-design.md`'s images. Worth doing, not worth debating further.

## What's deliberately not here

No local Kubernetes (`kind`/`minikube`) mirroring the lab cluster — compose is the right altitude for a two-service Go project's local loop; a local k8s cluster would be solving a problem this project doesn't have.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from 03's config surface and the containerization design above. Bree's and Callum's per-deliverable notes pending.*
