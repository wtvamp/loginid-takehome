# LT-46 gap: Postgres itself consumes four of the other five secrets as env vars

## The gap

`refinement/LT-46.md` frames each of the six application secrets as consumed
by one workload each (e.g. `db-runtime-credential` -> `api-service`). Checking
`deploy/manifests.yaml` directly (not assumed from the enumeration) shows the
`postgres` StatefulSet's own container env is wired from **five** of the six
secrets, not one:

```
POSTGRES_PASSWORD      <- postgres-superuser
MIGRATOR_PASSWORD      <- db-migrator-credential
RUNTIME_PASSWORD       <- db-runtime-credential
ISSUER_MIGRATOR_PASSWORD <- issuer-db-migrator-credential
ISSUER_RUNTIME_PASSWORD  <- issuer-db-runtime-credential
```

All five feed `postgres-initdb`'s `01-create-roles.sh` script (the
`docker-entrypoint-initdb.d` `ConfigMap`), which creates the `migrator`/
`runtime` roles (main and issuer databases) with these exact passwords —
they have to match what the migration `Job` and `api-service`/
`api-service-issuer` will later present, or authentication fails the first
time the database is actually used. This isn't a mistake in the current
Kubernetes-`Secret` design (`secretKeyRef` into env vars works fine there) —
it's a real consequence of one Postgres instance provisioning roles for
credentials that other workloads also hold copies of.

## Why this matters for the Vault re-scope specifically

Vault Agent Injector's default (and simplest) delivery mechanism writes
secret material to **files** in a shared `emptyDir` volume
(`/vault/secrets/<name>` by convention) — it does not populate a
container's environment variables directly. `postgres`'s container command
here expects five plain env vars, not five files. Two different fixes,
one per credential shape, neither requiring Amber's real names to state
now:

1. **`POSTGRES_PASSWORD` -> `POSTGRES_PASSWORD_FILE`.** Confirmed via the
   Docker Official `postgres` image's own documentation: `POSTGRES_PASSWORD`
   has a first-class `_FILE`-suffixed alternative, `POSTGRES_PASSWORD_FILE`,
   read by the image's own entrypoint at container start — this is exactly
   the file-mount shape Vault Agent Injector produces, no wrapper script or
   custom code needed. **Recommended: use this one directly**, once a
   Vault role/path exists for it.
2. **`MIGRATOR_PASSWORD`/`RUNTIME_PASSWORD`/`ISSUER_MIGRATOR_PASSWORD`/
   `ISSUER_RUNTIME_PASSWORD` have no such image-provided equivalent** —
   they're read directly by this project's own `01-create-roles.sh`, not
   by the base image. Closing this means editing that script to read
   each value from its injected file (`$(cat /vault/secrets/<name>)`)
   instead of `$MIGRATOR_PASSWORD` etc. — a real, if small, code change
   to an existing script, not a pure manifest edit. **Recommended over
   the alternative of templating one combined env file and `source`-ing
   it**, since it needs no wrapper/entrypoint change and matches the
   file-read pattern (1) already uses for the superuser password — one
   delivery shape for all five values, not two.

Both are stated as recommendations now that (1) is confirmed against the
image's own documented behavior, not left as an open toss-up — but
neither is implemented in this draft (`deploy/manifests.yaml`'s postgres
env block is deliberately left untouched), since doing so means deciding
and writing real injected-file paths, which still wants Amber's actual
Vault paths/roles to exist first so the file paths correspond to
something real rather than another guess. Flagging for whoever finalizes
the `postgres` StatefulSet's Vault annotations (Theo, once Amber's
roles/paths exist), since it changes what Amber's Vault role for the
`postgres` `ServiceAccount` needs read access to: **all five** paths,
not just `postgres-superuser`'s own — the enumeration in
`refinement/LT-46.md` undercounts `postgres`'s own Vault role scope if
read as "one secret per workload."

## What does NOT change

The `migrator`/`runtime` separation, and main/issuer database separation,
are unaffected — this is purely about how `postgres`'s own container
receives values it already receives today, not a change to which
credentials exist or what they're scoped to.
