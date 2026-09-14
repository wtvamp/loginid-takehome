#!/usr/bin/env python3
"""LT-53: the demo console's Ingress route (path /demo) must resolve to
api-service and nothing else -- the page carries no credential (a
visitor pastes their own QA client credentials, fetched via the LT-51
runbook), so its only real risk is accidentally routing to the wrong
backend (e.g. a copy-paste of the /auth/token rule that forgets to
change the service name), not exposing a secret.

Fails CI if the Ingress ever defines a /demo path pointing at any
Service other than api-service, or if the /demo path disappears
entirely from the Ingress while still referenced elsewhere (a partial
edit that would silently 404 the demo route from ingress-nginx's
default backend, the same class of gap LT-39's routes hit once
already -- see the comment on the /profiles rule in
deploy/manifests.yaml).
"""
import re
import sys

MANIFEST_FILE = "deploy/manifests.yaml"
EXPECTED_SERVICE = "api-service"


def main() -> int:
    try:
        text = open(MANIFEST_FILE).read()
    except FileNotFoundError:
        print(f"::error::{MANIFEST_FILE} not found")
        return 1

    # Matches the exact flow-style path entry shape this file's Ingress
    # rules already use: {path: /demo, pathType: ..., backend: {service:
    # {name: <service>, port: {number: ...}}}}
    pattern = re.compile(
        r'\{path:\s*/demo,\s*pathType:\s*\w+,\s*backend:\s*\{service:\s*\{name:\s*([a-z0-9-]+),'
    )
    matches = pattern.findall(text)

    if not matches:
        print(
            "::error file=deploy/manifests.yaml::No Ingress rule found for path "
            "/demo -- LT-53's demo console route is missing entirely."
        )
        return 1

    wrong = [name for name in matches if name != EXPECTED_SERVICE]
    if wrong:
        print(
            f"::error file=deploy/manifests.yaml::/demo routes to {wrong}, "
            f"not exclusively to '{EXPECTED_SERVICE}' -- the demo console must "
            f"only ever route to api-service."
        )
        return 1

    print(f"/demo routes only to {EXPECTED_SERVICE} ({len(matches)} match(es)).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
