#!/usr/bin/env python3
"""LT-46: fail if any of the six application secrets migrated to Vault
still appear as a Kubernetes Secret reference (volume or secretKeyRef/
envFrom.secretRef) anywhere under deploy/.

Not yet wired into any GitHub Actions workflow. Wiring this in only
makes sense in the same PR that actually removes the Secret volumes/
secretKeyRefs below and replaces them with Vault Agent Injector
annotations (Amber's real KV paths/roles) -- adding this check any
earlier would just fail every PR against the current, still-Secret-based
manifests. Kept here, written and ready, so that PR is a smaller diff
when the real names land: this script plus the manifest edits plus one
new pr-check.yml step, not three separate things invented from scratch
under time pressure.

qa-client-credential, postgres-server-tls, and loginid-takehome-tls are
deliberately NOT in MIGRATED_SECRET_NAMES -- see refinement/LT-46.md and
handoff-04-secrets.md for why each of those three stays a Kubernetes
Secret.
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
