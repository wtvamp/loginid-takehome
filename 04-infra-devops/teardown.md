# Teardown

What to delete when this take-home's deployment is done with, and who's responsible for triggering it. Written because this deploys to Warren's real, shared production cluster (`decisions/deploy-path.md`), not an isolated sandbox — nothing about a real deployment there has a default end-of-life, so one has to be named explicitly. Per Tobias's (01) objection, accepted during LT-45/LT-47 refinement.

## What gets deleted

1. **Namespace `loginid-takehome`** (the app namespace) — deleting it removes every application resource this project created: both `api-service`/`api-service-issuer` Deployments, `idp-connector`, both `Service`s, the `Ingress` (and its `cert-manager`-issued `Secret`), the `NetworkPolicy`, the `ResourceQuota` (`kubectl delete namespace loginid-takehome`). One command, one blast radius, by design (`containerization-design.md` §3's namespace scoping is exactly what makes this a one-liner instead of a manifest-by-manifest teardown).
2. **Namespace `loginid-takehome-runners`** (the runner-set namespace — separate from #1, don't assume deleting one covers the other) — `helm uninstall loginid-takehome-runners -n loginid-takehome-runners` removes the `AutoscalingRunnerSet`/listener/RBAC Helm installed; deleting the namespace after covers the `loginid-takehome-github-auth` Secret and the `loginid-takehome-deployer` ServiceAccount. **Also required, and not a Kubernetes step:** uninstall GitHub App `loginid-takehome-arc` (App ID 4926691) from `wtvamp/loginid-takehome` in Warren's GitHub App settings — deleting the Kubernetes Secret does nothing to the App installation itself, which remains valid until explicitly uninstalled.
3. **DNS record** — the `loginid-takehome.uplifttech.org` CNAME (added via Amber/OpenClaw against Bluehost, per the PM's relayed request). Not reversible from this track's tooling; whoever added it removes it the same way.
4. **GHCR images** (`ghcr.io/wtvamp/loginid-takehome/api-service`, `.../idp-connector`) — public images pushed on every merge to main. Low-consequence to leave (no secrets in them, per this track's own container design), but a full teardown deletes the packages too if the repo itself is being taken down.
5. **Not deleted by this teardown:** the GitHub repo itself, the ARC controller (`arc-system`) or the cluster's pre-existing general-purpose runner set (`arc-runners`) — neither was created by this project.

## Owner

This track (04/Theo) owns triggering steps 1, 2, and 4 (they're this track's own resources). Step 3 is whoever holds the Bluehost/DNS relationship — currently Amber/OpenClaw, via the PM.

## Trigger condition

**Not decided here — Warren's call.** Candidates a future decision could pick from: a fixed date (e.g., N days after the take-home is submitted/reviewed), an explicit "done reviewing" signal from Warren, or never (if he decides to keep it running as a portfolio piece). This document only guarantees that when the trigger fires, there's a named, complete list to execute against — it doesn't set the trigger itself.
