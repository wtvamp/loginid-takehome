#!/usr/bin/env python3
"""LT-46: fail if any of the six application secrets migrated to Vault
still appear as a Kubernetes Secret reference (volume or secretKeyRef/
envFrom.secretRef) anywhere under deploy/.

Wired into .github/workflows/pr-check.yml's no-vault-migrated-secrets
job as of the PR that also removed the Secret volumes/secretKeyRefs
below and added the real Vault Agent Injector annotations (Amber's
provisioning, 04-infra-devops/handoff-amber-vault-provisioning.md).

qa-client-credential, postgres-server-tls, and loginid-takehome-tls are
deliberately NOT in MIGRATED_SECRET_NAMES -- see refinement/LT-46.md and
handoff-04-secrets.md for why each of those three stays a Kubernetes
Secret. postgres-superuser WAS a fourth accepted exception in an
earlier version of this list (its own bootstrap-ordering rationale --
Postgres needing its password before anything else existed to serve
one) but the PM's ruling on 2026-09-13 retired that exception now that
Vault is confirmed running on this cluster before Postgres ever starts;
it's included in MIGRATED_SECRET_NAMES below like the other five.
"""
import re
import sys

MIGRATED_SECRET_NAMES = [
    "api-service-jwt-signing-key",
    "postgres-superuser",
    "db-migrator-credential",
    "db-runtime-credential",
    "issuer-db-migrator-credential",
    "issuer-db-runtime-credential",
]

MANIFEST_FILES = [
    "deploy/manifests.yaml",
    "deploy/api-service.yaml",
]


def find_references(text: str, name: str) -> list[str]:
    # Matches both forms this project's manifests actually use:
    #   secretName: <name>            (volumes: - secret: {secretName: ...})
    #   secretKeyRef: {name: <name>   (env: valueFrom.secretKeyRef)
    # A bare name match would also flag this file's own docstring/constant
    # list, so the pattern requires the YAML key context around it.
    pattern = re.compile(
        rf"(?:secretName|secretKeyRef:\s*\{{\s*name)\s*[:=]?\s*[\"']?{re.escape(name)}\b"
    )
    return pattern.findall(text)


def main() -> int:
    failed = False
    for path in MANIFEST_FILES:
        try:
            text = open(path).read()
        except FileNotFoundError:
            continue
        for name in MIGRATED_SECRET_NAMES:
            if find_references(text, name):
                print(
                    f"::error file={path}::Found a Kubernetes Secret reference to "
                    f"'{name}', which LT-46 requires be delivered via Vault Agent "
                    f"Injector instead (refinement/LT-46.md)."
                )
                failed = True
    if failed:
        return 1
    print("No migrated-secret references found in deploy/ manifests.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
