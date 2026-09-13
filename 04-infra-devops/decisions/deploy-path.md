# Deploy Path — Discovery Findings

Written per team-lead's agile-loop brief. Answers: how do we reach the cluster, how does GitHub Actions get code onto it, what does the public URL look like.

## 1. What the cluster actually is — correction to the planning-phase narrative

The planning-phase docs (`../PLAN.md`, `handoff-04-secrets.md`) described the deployment target as a "lab/dev cluster" reachable only through an external operator agent ("amber-kubernetes"). Direct discovery on this machine found something more specific and more consequential:

- This machine already holds a working `kubectl` context (`dev`, cluster `uplift-kub`, 5 nodes, `kubeconfig` at `~/.kube/config`) with direct access — no SSH hop, no operator agent needed to *reach* it.
- The "OpenClaw CI/CD in Kubernetes dev cluster" session named in the brief is not currently listed by `ListAgents` — not contacted, since direct access already exists and answers the question it would have answered.
- **This is a real, live, multi-tenant cluster, not an isolated sandbox.** Namespaces include `bitwarden` (Warren's actual password manager, `bitwarden.uplifttech.org`), `homeassistant`, `kafka`, `cert-manager`, `backup`, and several real business sites (`salemforensics.com`, `mc.uplifttech.org`/`acrimoniousmc.com`, `calcom.uplifttech.org`, `floracore.uplifttech.org`). All public-facing services follow one pattern: an `Ingress` (class `nginx`) under a real hostname, TLS via `cert-manager`'s `letsencrypt-prod` `ClusterIssuer`, routed through one shared `ingress-nginx-controller` `Service` (`LoadBalancer`, internal IP `192.168.1.200`, presumably port-forwarded to the internet at the router — not verified from here, and not something to change).
- **Confirmed with Warren directly before proceeding** (this track paused and asked, given the mismatch between the planning-phase "isolated lab cluster" framing and what discovery found): deploy to this real cluster, in an isolated namespace, with a public tunnel/URL, is approved.

**Consequence for this design:** every prior "lab cluster, nothing at stake" assumption is replaced with "shared production infrastructure serving real, unrelated services — isolate aggressively." Namespace, RBAC, and resource limits below are sized for that, not for a sandbox.

## 2. GitHub Actions → cluster: self-hosted runner already exists

GitHub-hosted runners can't reach a LAN cluster; the choice is a self-hosted runner or GitOps pull. **This cluster already runs Actions Runner Controller (ARC):**
- `arc-system` namespace: `arc-gha-rs-controller` (the ARC controller itself).
- `arc-runners` namespace: an `AutoscalingRunnerSet` named `k8s-linux-x64` (min 1, max 4 runners), with one runner Pod live.

This runner set's current GitHub registration target (which org/repo it's scoped to) isn't determined from here without reading a Secret, and isn't assumed to already point at `wtvamp/loginid-takehome` — that's a question for whoever manages this ARC install, not something to touch. **Decision: do not repurpose the existing runner set.** Deploying this take-home's images and manifests needs a `kubectl apply` from inside the cluster's network with a scoped ServiceAccount — reusing an already-provisioned runner set that may serve other repos/workloads would mean this take-home's deploy job runs with whatever permissions that runner already has, which is exactly the shared-blast-radius problem this project's own designs (NetworkPolicy, RBAC-per-service) have been arguing against. Instead: **a second, narrowly-scoped `AutoscalingRunnerSet`**, registered to `wtvamp/loginid-takehome` specifically, in its own namespace, with a ServiceAccount whose RBAC is limited to the take-home's own namespace only (`get`/`list`/`create`/`update`/`patch` on `Deployments`/`Services`/`Ingresses`/`NetworkPolicies`/`Secrets`-by-name in that namespace, nothing cluster-scoped). This is additive to the cluster's existing ARC controller (one controller can run multiple scale sets), not a competing install.

## 3. Namespace and isolation

New namespace: `loginid-takehome`. Everything in `./containerization-design.md`'s manifests (both Deployments, the issuer Deployment, both Services, the `NetworkPolicy`) targets this namespace, not the placeholder `loginid-poc` used in the design-phase sketch — that name gets corrected in this pass. Resource limits from `./containerization-design.md` stay as designed (they were sized for "a two-service take-home," which is even more clearly correct now that the cluster is shared with real workloads, not less). A `ResourceQuota` on the namespace is a cheap additional backstop worth adding given the shared-cluster finding — caps total CPU/memory the whole take-home can ever consume regardless of what any single manifest requests.

## 4. Public URL

Following the cluster's existing, uniform pattern rather than inventing a new one (a Cloudflare Tunnel or similar was the planning-phase's guess; the cluster doesn't use one — every existing public service is a plain `Ingress` + `cert-manager` + the shared `ingress-nginx` `LoadBalancer`):

- **Ingress host:** `loginid-takehome.uplifttech.org`, `Ingress` class `nginx`, TLS via `letsencrypt-prod`, matching every other app in the cluster.
- **Open item, not resolvable from this session:** the DNS record for that hostname doesn't currently resolve (checked; no answer). Either it needs adding at whatever DNS provider hosts `uplifttech.org` (Warren's call — this track has no DNS credentials and isn't requesting them) or it already resolves via a wildcard this check didn't hit correctly; needs one confirmation before the first deploy's public URL is real rather than theoretical. Flagging to Naomi/PO and Dana now rather than discovering it at the acceptance-script step.
- **Routes:** `/api/*` (or a path prefix TBD with 03) → `api-service`; the issuer's endpoints (`/auth/token`, `/.well-known/jwks.json` or similar, per 02/03's final route naming) → `api-service-issuer`. `idp-connector` gets no route — it's never publicly reachable, per its own design.
- **Acceptance-script shape this enables:** `https://loginid-takehome.uplifttech.org/healthz` (and an equivalent connector-side path if 03 exposes one) returning up + the deployed commit SHA, per team-lead's target script — `/healthz` already exists in the containerization design and 03's `internal/app` build-info work (per PM's note on LT-34) is what supplies the SHA.

## 5. What this changes in `./containerization-design.md` (applied in this pass)

- `namespace: loginid-poc` → `namespace: loginid-takehome` throughout.
- New `Ingress` resource (host above, TLS via `letsencrypt-prod`) added alongside the existing Services.
- A namespace-level `ResourceQuota` added as a backstop.
- Everything else (two Deployments, the issuer split, `NetworkPolicy`, resource limits per container, the migration `Job`) is unchanged — those were designed correctly for "shared infrastructure, isolate aggressively" even under the earlier, wrong assumption that this was a private sandbox.

## 5a. Deploy stage: full auto-deploy on merge, no manual gate

For this take-home's scale, the merge-to-main workflow deploys automatically — no manual approval gate on the actual deploy step. Recorded here (not just in chat to Naomi/01) since it changes LT-47's acceptance criterion from "documented but not invoked" (the design-phase framing) to "invoked on every merge."

## 6. CI/CD sequencing (per team-lead's split)

The PR-check workflow (lint, build, `-race` tests, `govulncheck`) needs none of the above and ships first, on GitHub-hosted runners — no cluster access required. The merge-to-main workflow (build+push images to GHCR, apply manifests via the new scoped runner set, print the public URL + commit SHA in the job summary) follows once the runner set and namespace exist. Both live at `.github/workflows/` at the repo root.

---
*AI tooling note: discovery run directly by Theo (Sonnet, this session) via `kubectl`, `dig`, and `ListAgents` — no external operator agent contact was needed or made. Confirmed with Warren directly (AskUserQuestion) before treating the real-cluster finding as a green light, given the mismatch with the planning-phase framing.*
