#!/usr/bin/env bash
# LT-48: writes .dev-secrets/* with placeholder, local-only dev values.
# Safe to re-run — skips any file that already exists, so a developer's
# own edits (e.g. a real local Postgres password they set) aren't
# clobbered on a second run.
#
# .dev-secrets/ is .gitignore'd. These values are never used outside a
# developer's own machine and are never the pattern used in
# 04-infra-devops/containerization-design.md or secrets-delivery.md —
# see local-dev-loop.md's own "Dev credentials are dev credentials"
# section.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dir="$root/.dev-secrets"
mkdir -p "$dir"

# ./.dev-data (docker-compose.yml's api-service/api-service-dev bind
# mount for /data): world-writable, since the production image
# (gcr.io/distroless/static-debian12:nonroot) never creates /data or
# chowns it before switching to a non-root user — a fresh Docker named
# volume mounted there would be root-owned and unwritable by the
# container. A bind-mounted, chmod 777 host directory sidesteps that
# regardless of which UID the container runs as; this is throwaway
# local dev data, not a security-sensitive artifact.
mkdir -p "$root/.dev-data"
chmod 777 "$root/.dev-data"

write_if_absent() {
  local file="$1" value="$2"
  if [ -f "$file" ]; then
    echo "skip (exists): $file"
  else
    printf '%s' "$value" > "$file"
    chmod 600 "$file"
    echo "wrote: $file"
  fi
}

# Two DSN files, not one — Naomi's (01) live re-review found the
# postgres profile failing healthz with db: down: a single db_dsn file
# written unconditionally as the SQLite path meant api-service-pg's
# DB_DSN_FILE (DB_DRIVER=postgres) pointed at a sqlite-shaped DSN
# regardless of which profile actually ran this script last or which
# profile is active now. Fixed the fragile way first drafts reach for
# (branch on which profile the developer says they're using) by
# removing the branch entirely instead: write BOTH DSNs unconditionally,
# and let each profile's own compose service definition point its
# DB_DSN_FILE at the one it actually needs (api-service ->
# db_dsn.sqlite, api-service-pg -> db_dsn.postgres) — nothing depends
# on script invocation order or which profile ran last.
#
# Plain file path, not a "file:" URI, for the sqlite one —
# internal/dao/sqlite.New's dsn argument is a bare path or ":memory:"
# (internal/dao/sqlite/sqlite.go), confirmed by reading the driver
# directly rather than assumed from the design sketch's own comment,
# which used "file:/data/dev.db" — that form isn't what this driver
# actually parses.
write_if_absent "$dir/db_dsn.sqlite" "/data/dev.db"
dev_pg_password="dev-only-not-a-real-secret"
write_if_absent "$dir/postgres_password" "$dev_pg_password"
write_if_absent "$dir/db_dsn.postgres" "postgres://dev:${dev_pg_password}@postgres:5432/loginid_dev?sslmode=disable"

# LT-48 (Naomi's live review): the issuer's own database — same "dev"
# role/password as the main database (docker-postgres-initdb's
# 01-create-issuer-db.sh creates the second database, not a second
# role) — no migrator/runtime split locally, per local-dev-loop.md's
# own non-goal. sslmode=disable: TLS is a real-cluster concern
# (secrets-delivery.md), not this loop's.
write_if_absent "$dir/issuer_db_dsn" "postgres://dev:${dev_pg_password}@postgres:5432/loginid_dev_issuer?sslmode=disable"

# JWT signing key (internal/api/signingkey.go's LoadSigningKey accepts
# PKCS#1 or PKCS#8 PEM — openssl genrsa produces PKCS#1 directly,
# confirmed by reading parseRSAPrivateKey). Local-only key, generated
# fresh per developer machine, never the real cluster's signing key
# (deploy/issuer-secret-bootstrap.yaml's out-of-band mechanism) — this
# one only ever signs tokens api-service-issuer's own dev instance
# mints and api-service's own dev instance verifies, on one machine.
if [ -f "$dir/jwt_signing_key" ]; then
  echo "skip (exists): $dir/jwt_signing_key"
else
  openssl genrsa -out "$dir/jwt_signing_key" 2048 2>/dev/null
  chmod 600 "$dir/jwt_signing_key"
  echo "wrote: $dir/jwt_signing_key"
fi

echo
echo "Done. docker compose --profile sqlite up   (or --profile postgres, or --profile dev)"
echo "Then: ./scripts/dev-migrate-and-seed.sh    (once postgres is healthy — runs migrations, seeds a dev OAuth client)"
