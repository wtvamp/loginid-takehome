#!/usr/bin/env bash
# LT-48 (Naomi's live-review ruling): runs migrations against the local
# Postgres and seeds one dev OAuth client, then prints a ready-to-paste
# curl that mints a real token through the real APP_MODE=issuer code
# path — no dev-only bypass.
#
# Run once postgres is up and healthy (either --profile sqlite or
# --profile postgres — the issuer database is unconditionally Postgres
# regardless of which profile you're running the main DAO under, see
# docker-compose.yml's own comment on api-service-issuer).
#
# Migration scope/order matches the real cluster's own three-invocation
# order exactly (deploy/manifests.yaml's db-migrate Job) — caught live,
# not assumed: migrations/shared seeds auth_method, which belongs to
# the MAIN database's own schema (created by migrations/postgres) and
# has nothing to do with oauth_client — running it against the issuer
# database first (my own first draft of this script) fails immediately
# with "relation auth_method does not exist", since migrations/shared
# depends on a table only migrations/postgres creates. Corrected order:
# postgres/ then shared/ against the MAIN database, issuer/ against the
# ISSUER database, three separate invocations, never combined.
#
# Scope note: if you're on --profile sqlite, the main database's own
# schema (migrations/sqlite + migrations/shared) still needs creating
# against .dev-data/dev.db — run goose locally yourself (`go run
# github.com/pressly/goose/v3/cmd/goose@v3.22.1 sqlite3
# .dev-data/dev.db -dir migrations/sqlite up`, then the same -dir
# migrations/shared) rather than through the containerized migrate
# tool: goose-in-a-container against a bind-mounted SQLite file wasn't
# verified to work with this project's pure-Go sqlite driver in the
# time available for this fix, and guessing at that wasn't worth
# risking over the actually-reported bug (the issuer not existing at
# all). This script handles the --profile postgres main-DB path plus
# the issuer database, which is needed either way to mint a token.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

main_dsn="postgres://dev:dev-only-not-a-real-secret@postgres:5432/loginid_dev?sslmode=disable"
issuer_dsn="postgres://dev:dev-only-not-a-real-secret@postgres:5432/loginid_dev_issuer?sslmode=disable"

if [ "${1:-}" = "--postgres-profile" ]; then
  echo "Running main-database migrations (migrations/postgres, migrations/shared)..."
  docker compose run --rm \
    -e GOOSE_DRIVER=postgres \
    -e GOOSE_DBSTRING="$main_dsn" \
    -e GOOSE_MIGRATION_DIR=/migrations/postgres \
    migrate up
  docker compose run --rm \
    -e GOOSE_DRIVER=postgres \
    -e GOOSE_DBSTRING="$main_dsn" \
    -e GOOSE_MIGRATION_DIR=/migrations/shared \
    migrate up
fi

echo "Running issuer migrations (migrations/issuer, against the ISSUER database only)..."
docker compose run --rm \
  -e GOOSE_DRIVER=postgres \
  -e GOOSE_DBSTRING="$issuer_dsn" \
  -e GOOSE_MIGRATION_DIR=/migrations/issuer \
  migrate up

echo "Seeding one dev OAuth client (client_id: dev-client)..."
dev_secret="dev-client-secret-not-real"
dev_hash="$(go run deploy/gen-argon2-hash.go "$dev_secret")"

docker compose exec -T postgres env PGPASSWORD=dev-only-not-a-real-secret psql \
  -U dev -d loginid_dev_issuer -v ON_ERROR_STOP=1 -c "
    INSERT INTO oauth_client (client_id, name, client_secret_hash, secret_state, granted_scopes, audience, status)
    VALUES ('dev-client', 'Local dev client', '${dev_hash}', 'set', ARRAY['profile:read:own'], 'loginid-api-service', 'active')
    ON CONFLICT (client_id) DO UPDATE SET client_secret_hash = EXCLUDED.client_secret_hash, secret_state = 'set', status = 'active';
  "

echo
echo "Done. Mint a token:"
echo
cat <<EOF
curl -u dev-client:${dev_secret} \\
  -d 'grant_type=client_credentials' \\
  http://localhost:8082/auth/token
EOF
