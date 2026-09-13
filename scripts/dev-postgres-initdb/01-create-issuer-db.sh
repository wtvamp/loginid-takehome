#!/bin/bash
# Runs once, only on an empty $PGDATA (docker-entrypoint-initdb.d's own
# behavior, not a choice this script makes) — creates the second
# database the issuer (APP_MODE=issuer) connects to. Same "dev" role
# and password as the main database; local dev doesn't need the
# migrator/runtime privilege split the real cluster's postgres-initdb
# ConfigMap enforces (local-dev-loop.md's own "no production credential
# handling review" non-goal) — one dev superuser is enough for a
# single-developer, single-instance local loop.
set -euo pipefail
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -c "CREATE DATABASE loginid_dev_issuer;"
