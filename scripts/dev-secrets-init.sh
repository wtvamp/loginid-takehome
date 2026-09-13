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

# Plain file path, not a "file:" URI — internal/dao/sqlite.New's dsn
# argument is a bare path or ":memory:" (internal/dao/sqlite/sqlite.go),
# confirmed by reading the driver directly rather than assumed from the
# design sketch's own comment, which used "file:/data/dev.db" — that
# form isn't what this driver actually parses.
write_if_absent "$dir/db_dsn" "/data/dev.db"
write_if_absent "$dir/postgres_password" "dev-only-not-a-real-secret"

echo
echo "Done. docker compose --profile sqlite up   (or --profile postgres, or --profile dev)"
