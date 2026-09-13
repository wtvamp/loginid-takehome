# Vault Agent Injector failure-mode behavior (LT-46, Tobias's fail-loud criterion)

## What this answers

`refinement/LT-46.md`'s added acceptance criterion: a pod whose Vault Agent init-container cannot authenticate or fetch its secret within a bounded timeout must fail **visibly** — `CrashLoopBackOff` with a distinguishing signal, not an indefinite silent `Init` hang.

## Cluster fact-check first

This cluster already runs `hashicorp/vault-k8s:1.7.0` as the injector (`vault-agent-injector` Deployment in the `vault` namespace, confirmed via `kubectl get pods -n vault -o jsonpath`), and Vault itself (3-node raft, initialized, unsealed) has been running here for 571+ days — this is existing shared infrastructure, not something LT-46 stands up. What's missing is project-specific: KV paths and per-`ServiceAccount` Kubernetes-auth roles for `loginid-takehome`'s six application secrets, which is Amber's in-progress provisioning work.

## Correction (2026-09-13): the original conclusion below was wrong, found live by Naomi

Naomi's PO-review step 6, run for real once Amber's roles existed: a throwaway pod under the
`api-service` ServiceAccount pointed at the `loginid-takehome-postgres` role got a correct `403`
from Vault — but `vault-agent-init` never exited. It sat `Init:0/1`, zero restarts, for seven
minutes, with the auth backoff climbing (26s, 46s, ...) toward Vault Agent's own 5-minute default
ceiling. No `CrashLoopBackOff`, no terminal state — exactly the indefinite silent `Init` hang this
criterion exists to rule out, and the doc's "no manifest annotation is needed" conclusion below was
the direct cause: it was wrong, not merely unverified.

**Root cause: two separate retry loops inside Vault Agent, only one of which was covered here.**

- `client-max-retries`/`template-config-exit-on-retry-failure` (the two annotations this doc
  originally cited) govern **secret-fetch/template-rendering retries** — what happens *after* the
  agent has already authenticated to Vault, while it's rendering a KV read into a file.
- **`auto_auth`'s own retry loop governs the *authentication* step itself** (the Kubernetes-auth
  login call, `vault write auth/kubernetes/login role=...`) — a completely separate code path this
  doc never addressed. A denied role produces an auth failure, not a template-rendering failure, so
  the two annotations already documented here never applied to it at all.

Auto-auth's own defaults, confirmed against HashiCorp's `autoauth` reference: **`exit_on_err`
defaults to `false`** (retries forever, by design — reasonable for a transient network blip against
a real Vault, wrong for a permission denial that will never resolve on its own), with
`min_backoff`/`max_backoff` defaulting to **1s/5m** — exactly matching the climbing 26s/46s/...
backoff Naomi observed heading toward that 5-minute ceiling, never reached within her seven-minute
observation window because `exit_on_err: false` means there is no ceiling that ever triggers an exit
in the first place.

## Fix: two more annotations, on all five injected workloads

- `vault.hashicorp.com/agent-auto-auth-exit-on-err: "true"` — the actual switch that makes an
  auth failure terminal instead of retried forever (confirmed against HashiCorp's own docs: even
  with this set to `true`, Vault Agent still retries with backoff before exiting — it does not exit
  on the very first failed attempt — so this alone doesn't bound *how long* the pod hangs before
  going terminal).
- `vault.hashicorp.com/auth-max-backoff: "10s"` — bounds that backoff ceiling down from the 5-minute
  default, so the handful of retries `exit_on_err` still performs before giving up complete in well
  under a minute rather than climbing toward 5 minutes per attempt.

Added to `api-service`, `api-service-issuer`, `postgres`, `db-migrate`, and `retention-sweep` — every
workload with `vault.hashicorp.com/agent-inject: "true"` set at all, not just the one Naomi's test
happened to exercise.

## What that means for pod state (standard Kubernetes init-container semantics, not Vault-specific)

An init container that exits non-zero is retried by the kubelet according to the pod's `restartPolicy`:

- **Deployments/StatefulSets** (`restartPolicy: Always` — `api-service`, `api-service-issuer`, `postgres`): a failing `vault-agent-init` container, once it actually exits (which needed the auto-auth fix above — it never did before), drives the pod through `Init:Error` into `Init:CrashLoopBackOff`, visible in `kubectl get pods` and as pod Events — exactly the signal this criterion asks for. `readinessProbe`/`livenessProbe` never even get a chance to run, since the main container never starts — there's no route to a "silently serving with stale/absent secrets" outcome.
- **`db-migrate`/`retention-sweep`** (`restartPolicy: Never`): a failing init container fails the pod outright; the Job's own `backoffLimit` governs retry count the same way it already does for a `goose`/sweep failure, then the Job reports `Failed` — which the deploy workflow already polls for explicitly (word-matching `Complete`/`Failed`, not `kubectl wait`, per the earlier DNS-race hardening work) and fails the deploy loudly rather than hanging.

## What this doc still does not establish

The auto-auth fix above is stated from HashiCorp's own `autoauth`/injector-annotations reference (confirmed both the `exit_on_err` default and the still-retries-before-exiting behavior directly against the docs, not assumed) — it has not yet been re-run live against this cluster's actual injector. That live confirmation is the same PO-review-script step 6 that found the original bug: a throwaway pod on a denied role must reach `Init:CrashLoopBackOff`, not hang, once this fix deploys. LT-46 stays In Test until Naomi re-runs it and it passes.

## Conclusion

The original version of this doc concluded no manifest annotation was needed for the fail-loud criterion, reasoning only from the template-rendering retry annotations. That conclusion covered one of Vault Agent's two independent retry loops and missed the other — auto-auth's own, which defaults to retrying forever with no exit at all. Both `agent-auto-auth-exit-on-err` and `auth-max-backoff` are now required annotations on every injected workload, not optional defaults-already-correct ones. The lesson generalizes: "the defaults already do the right thing" is a claim to verify per failure mode, not per component — this component (Vault Agent) has more than one failure mode, and checking one doesn't clear the others.
