#!/usr/bin/env python3
"""Backlog Story (LT-44 hotfix, Naomi's live review): every workload
that holds a database credential must appear in postgres-default-deny's
ingress allow-list, or it can authenticate to Postgres with a real
credential and still be silently dropped at the network layer -- the
retention-sweep bug (a TCP socket stuck in SYN_SENT, nothing in
pg_stat_activity, no error from Vault or the app) that this check exists
to make impossible for a sixth consumer to repeat.

"Holds a database credential" is detected two ways, matching how this
project's own manifests actually deliver one:
  (a) a DB_DSN_FILE or ISSUER_DB_DSN_FILE env var (api-service's own
      pattern), or
  (b) a Vault agent-inject-secret-* annotation whose value references
      one of the four DB credential KV paths (db-migrator-credential,
      db-runtime-credential, issuer-db-migrator-credential,
      issuer-db-runtime-credential) -- covers db-migrate/retention-sweep,
      which read these as files, not DB_DSN_FILE-named env vars.

The postgres workload itself is excluded: it's the NetworkPolicy's own
podSelector target (the server), not a source needing ingress to itself,
even though its own Vault role reads all four+one DB credentials.
"""
import re
import sys

MANIFEST_FILES = [
    "deploy/manifests.yaml",
    "deploy/api-service.yaml",
]

DB_CREDENTIAL_PATH_FRAGMENTS = [
    "db-migrator-credential",
    "db-runtime-credential",
    "issuer-db-migrator-credential",
    "issuer-db-runtime-credential",
]

DB_ENV_VAR_NAMES = ["DB_DSN_FILE", "ISSUER_DB_DSN_FILE"]

WORKLOAD_KINDS = re.compile(r"^kind:\s*(Deployment|StatefulSet|CronJob|Job)\s*$", re.MULTILINE)
APP_LABEL = re.compile(r"labels:\s*\{app:\s*([a-z0-9-]+)\}")


def split_docs(text: str) -> list[str]:
    return text.split("\n---\n")


def doc_app_name(doc: str) -> str | None:
    m = APP_LABEL.search(doc)
    return m.group(1) if m else None


def doc_has_db_credential(doc: str) -> bool:
    if any(f"{{name: {name}," in doc or f"name: {name}\n" in doc for name in DB_ENV_VAR_NAMES):
        return True
    if any(fragment in doc for fragment in DB_CREDENTIAL_PATH_FRAGMENTS):
        return True
    return False


def find_postgres_allowlist(docs: list[str]) -> set[str]:
    for doc in docs:
        if 'name: postgres-default-deny' in doc:
            return set(re.findall(r"podSelector:\s*\{matchLabels:\s*\{app:\s*([a-z0-9-]+)\}\}", doc))
    return set()


def main() -> int:
    all_docs = []
    for path in MANIFEST_FILES:
        try:
            all_docs.extend(split_docs(open(path).read()))
        except FileNotFoundError:
            continue

    required = set()
    for doc in all_docs:
        if not WORKLOAD_KINDS.search(doc):
            continue
        app = doc_app_name(doc)
        if app is None or app == "postgres":
            continue
        if doc_has_db_credential(doc):
            required.add(app)

    allowlist = find_postgres_allowlist(all_docs)
    missing = required - allowlist

    if missing:
        print(
            "::error file=deploy/manifests.yaml::The following workload(s) hold a "
            "database credential (DB_DSN_FILE/ISSUER_DB_DSN_FILE or a Vault-delivered "
            "DB credential path) but are missing from postgres-default-deny's ingress "
            "allow-list -- they can authenticate to Postgres but the NetworkPolicy will "
            "silently drop every packet:"
        )
        for name in sorted(missing):
            print(f"  - {name}")
        return 1

    print(f"postgres-default-deny covers every DB-credential-holding workload: {sorted(required)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
