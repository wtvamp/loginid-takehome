# Vault Agent Injector failure-mode behavior (LT-46, Tobias's fail-loud criterion)

## What this answers

`refinement/LT-46.md`'s added acceptance criterion: a pod whose Vault Agent init-container cannot authenticate or fetch its secret within a bounded timeout must fail **visibly** — `CrashLoopBackOff` with a distinguishing signal, not an indefinite silent `Init` hang.

## Cluster fact-check first

This cluster already runs `hashicorp/vault-k8s:1.7.0` as the injector (`vault-agent-injector` Deployment in the `vault` namespace, confirmed via `kubectl get pods -n vault -o jsonpath`), and Vault itself (3-node raft, initialized, unsealed) has been running here for 571+ days — this is existing shared infrastructure, not something LT-46 stands up. What's missing is project-specific: KV paths and per-`ServiceAccount` Kubernetes-auth roles for `loginid-takehome`'s six application secrets, which is Amber's in-progress provisioning work.

## Documented default behavior (HashiCorp's own injector-annotations reference, not assumed)

Two annotations govern this, both defaulting to the fail-loud direction already:

- `vault.hashicorp.com/client-max-retries` — defaults to **2**, for **3 total attempts**, before giving up.
- `vault.hashicorp.com/template-config-exit-on-retry-failure` — defaults to **true**: after those attempts are exhausted, the Vault Agent process **exits** rather than continuing to retry indefinitely in the background.

Neither annotation needs to be set for this project — both defaults already point the right way. Nothing here requires a real Vault role/path name to state or verify, which is why this is written up now rather than waiting on Amber.

## What that means for pod state (standard Kubernetes init-container semantics, not Vault-specific)

An init container that exits non-zero is retried by the kubelet according to the pod's `restartPolicy`:

- **Deployments/StatefulSets** (`restartPolicy: Always` — `api-service`, `api-service-issuer`, `postgres`): a failing `vault-agent-init` container drives the pod through `Init:Error` into `Init:CrashLoopBackOff`, visible in `kubectl get pods` and as pod Events — exactly the signal this criterion asks for, and standard Kubernetes behavior once the Agent itself exits per the defaults above. `readinessProbe`/`livenessProbe` never even get a chance to run, since the main container never starts — there's no route to a "silently serving with stale/absent secrets" outcome.
- **The migration `Job`** (`restartPolicy: Never`): a failing init container fails the pod outright; the Job's own `backoffLimit: 2` (unchanged by LT-46 — Story 8's DNS-wait `initContainer` is a separate, earlier init container in the same pod, ahead of the Vault one in the ordered list) governs retry count the same way it already does for a `goose` failure, then the Job reports `Failed` — which the deploy workflow already polls for explicitly (word-matching `Complete`/`Failed`, not `kubectl wait`, per the earlier DNS-race hardening work) and fails the deploy loudly rather than hanging.

## What this doc does NOT establish

This is documented behavior from HashiCorp's own reference, cross-checked against ordinary Kubernetes init-container semantics — not yet an empirical reproduction against *this* cluster's injector and *this* project's eventual Vault roles. A live scratch-namespace test (deliberately pointing a role at a nonexistent Vault path, then watching `kubectl get pods`/events) was the natural next step, but creating cluster-mutating scratch resources for this was denied by the session's own permission classifier as a shared-cluster mutation outside this task's explicit scope — correctly so, since it wasn't something Warren had approved in advance. That live confirmation is `refinement/LT-46.md`'s PO-review-script step 6, to run once Amber's real roles/paths exist and can be deliberately broken for the test — not skipped, just correctly sequenced after real names exist rather than against fabricated placeholder ones.

## Conclusion

No manifest annotation is needed purely to satisfy the fail-loud criterion — the injector's shipped defaults already produce `Init:CrashLoopBackOff` (Deployments/StatefulSets) or a failed Job (bounded by the existing `backoffLimit`) on sustained auth/fetch failure. What LT-46's manifest work still needs from Amber is the actual `vault.hashicorp.com/agent-inject: "true"`, role, and path annotations themselves — those enable the mechanism this doc describes, they don't need to additionally configure the failure behavior, which is already correct out of the box.
