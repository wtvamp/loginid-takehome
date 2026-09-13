# Hand-off: what observability the cluster already has (LT-49, LT-20)

**From:** Amber (OpenClaw Kubernetes agent on the Mac mini), read-only inventory on Warren's instruction, 2026-09-13 11:42–11:57 PDT. **Requested by:** Dana Whitfield (PM), after Warren asked "don't we already have a logging/monitoring/observability stack in the cluster?" **Receivers:** Theo Bergman (04) for the sink, alert rules, NetworkPolicy and RBAC; Renata Cole (03) for the `/metrics` exposure; Priya Nandakumar (05) for the sweep-result persistence contract; Naomi Voss (01) for the LT-49 and LT-20 refinement records; Marcus Ilori (02) for the audit-log residual.

Amber's report follows verbatim. Names, versions, and namespaces only; nothing was created or changed.

---

## 1. What's actually here

Both a Prometheus stack and an Elastic stack, from different eras and owned by different operators.

| Component | Version | Namespace | Owner |
|---|---|---|---|
| kube-prometheus-stack operator | — | `monitoring` | Helm release `prometheus` |
| Prometheus | `v3.13.2-distroless` | `monitoring` | Prometheus Operator |
| Alertmanager | — | `monitoring` | Prometheus Operator |
| Grafana | `13.1.1` | `monitoring` | Helm |
| kube-state-metrics + node-exporter (DaemonSet, 5/5) | — | `monitoring` | Helm |
| ECK operator | — | `elastic-system` | ECK |
| Elasticsearch (CR `elastic`, 1 node, **yellow**) | `9.4.4` | `logging` | ECK |
| Kibana (CR `kibana`) | `9.4.4` | `logging` | ECK |
| Filebeat DaemonSet (5/5) | `9.4.4` | `logging` | **Helm, not ECK** |

All `monitoring.coreos.com` CRDs are present: `prometheusrules`, `servicemonitors`, `podmonitors`, `scrapeconfigs`, `probes`, `alertmanagerconfigs`, `thanosrulers`.

**No Loki. No Fluent Bit.** Note the Filebeat is *not* an ECK `Beat` CR (`kubectl get beat -A` → none); it's a Helm DaemonSet configured by ConfigMap `filebeat-filebeat-daemonset-config` in `logging`. Theo was right to ask before touching `logging`, but the shared object he'd have had to edit is that ConfigMap, not the ES/Kibana CRs.

## 2. Yes — your stdout is already being collected

Filebeat tails `/var/log/containers/*.log` on every node with no namespace allowlist, and adds `add_kubernetes_metadata`. Your lines land in the data stream:

**`filebeat-9.4.4`** (backing indices `.ds-filebeat-9.4.4-YYYY.MM.DD-NNNNNN`, ~33M docs / 18.5 GB)

I confirmed rather than assumed — `loginid-takehome` already has **2,643 documents**, including `db-migrate`, `postgres-0`, `lt46-injector-failure-test{,2,3}`, `lt44-review-2`. Filter field is `kubernetes.namespace`. So (a)'s *storage* problem is already solved and costs you nothing.

**Two traps, both real:**

- ⚠️ Filebeat has `drop_event` on regex `(kube-probe|health|/healthz|/readyz)` against `message`. Any metric line containing the substring **"health"** is silently discarded — `retention_sweep_health 1` would simply never appear, with no error anywhere. Name your metrics around that.
- ⚠️ **There is no per-namespace tenancy pattern.** 29 ES roles, all built-in — zero custom roles. Kibana has only the `default` space. You would be the first tenant, so there's no precedent to copy.

## 3. Where alert rules live

**Prometheus/Alertmanager. Not Kibana, not Grafana.**

- 30+ `PrometheusRule` objects, all in `monitoring`, all from the Helm release.
- **Kibana alerting: 0 rules, 0 connectors.** Entirely unused — choosing it means building the rule engine *and* the notification path from scratch.
- Grafana has only Prometheus + Alertmanager datasources — **no Elasticsearch datasource**, so Grafana cannot query your logs today.

**The house self-service mechanism exists and is permissive.** The Prometheus CR:

```
ruleSelector            = {matchLabels: {release: prometheus}}
ruleNamespaceSelector   = {}        # empty = ALL namespaces
serviceMonitorSelector  = {matchLabels: {release: prometheus}}
serviceMonitorNamespaceSelector = {}
```

So **a `PrometheusRule` or `ServiceMonitor` in `loginid-takehome` carrying the label `release: prometheus` is adopted automatically**, with no edit to any shared object. Prometheus's ClusterRole+ClusterRoleBinding already grant cluster-wide `get/list/watch` on pods/services/endpoints, so it can scrape you.

Caveat I want to be straight about: all 13 existing ServiceMonitors are in `monitoring`. No tenant has used this path yet, so I'm verifying it from configuration, not from a working precedent.

⚠️ **Alertmanager will silently eat your low-severity rule.** The route tree is:

```
root receiver: discord    group_by: [namespace, alertname]
  ├─ alertname =~ Watchdog|InfoInhibitor  → null
  ├─ severity  =~ info|none               → null      ← dropped
  └─ severity  =  critical                → discord
  (no match → falls through to root → discord)
```

`RetentionSweepSkipRate` at `severity: info` goes to the **null receiver and is never delivered**. Use `severity: warning` — it falls through to `discord`. `group_by` already includes `namespace`, so your alerts group separately from everyone else's.

## 4. Recommendation

**The blocking constraint: Prometheus cannot ingest stdout.** It scrapes HTTP endpoints. And a CronJob pod is short-lived, so even a PodMonitor would usually miss the run entirely. Pushgateway is the normal answer to exactly this and you've ruled it out — so the metrics need a different door.

**(a) Export from the long-running pod, not from the CronJob.**

Have the sweep persist its results to the database it already has access to (`db-runtime-credential`), and have **api-service** expose them on an HTTP `/metrics` port as gauges — `retention_sweep_last_success_timestamp_seconds`, `oldest_surviving_row_age_seconds`, `retention_sweep_skipped_total`. Then one `ServiceMonitor` in your namespace labelled `release: prometheus`.

This isn't just a workaround — it's what makes rule (b)#1 *expressible*. "No fresh sample in > 2 intervals" becomes `time() - retention_sweep_last_success_timestamp_seconds > 2*interval`, which evaluates correctly **because an always-up pod serves the gauge**. If you instead alerted on the absence of a short-lived scrape target, you'd get an `absent()` that fires permanently and can never resolve.

Keep printing to stdout as well — it's already captured in ES for free and gives you the forensic trail.

⚠️ **NetworkPolicy will block the scrape.** `api-service-issuer`, `idp-connector` and `postgres` each have a default-deny ingress policy; none admits `monitoring`. (`api-service` currently has no policy, so it's reachable.) Whichever pod serves `/metrics` needs an ingress rule with `namespaceSelector: {kubernetes.io/metadata.name: monitoring}` — that label is auto-applied, so you select `monitoring` **without modifying it**. The policy object is yours, in your namespace.

**(b)** One `PrometheusRule` — e.g. `loginid-takehome-retention` in ns `loginid-takehome`, label `release: prometheus`, all three rules in it. `warning` not `info` for the skip-rate rule.

⚠️ **Your CI deployer cannot create either object yet.** `Role/loginid-takehome-deployer` grants `apps`, `""`, `networking.k8s.io`, `cert-manager.io`, `batch` — but **not `monitoring.coreos.com`**. Add that apiGroup with `prometheusrules`/`servicemonitors` to your own `deploy/rbac.yaml`. Namespaced Role, no shared object touched.

**(c) This one does not fit the model, and I'd rather say so than dress it up.**

The ES licence is **`basic`**. Document-level security is a Platinum/Enterprise feature, so **you cannot grant a filtered read over the shared `filebeat-9.4.4` stream restricted to `kubernetes.namespace: loginid-takehome`.** On Basic, index privileges are index-level only. Any role you create over `filebeat-9.4.4` grants the whole cluster's logs — every other tenant included.

Two honest options:

1. **Dedicated audit stream (recommended).** Have api-service write audit events to its own data stream (e.g. `loginid-takehome-audit-*`) rather than relying on stdout capture. Then an ES role granting `read` on just that pattern is genuinely scoped, with no DLS needed. The index is created by your app; only the role/user is a shared-stack object.
2. Accept namespace-wide log read on the shared stream — which I don't recommend for something labelled "audit".

Either way, note that an ES role + user is a **cluster-level object in the shared Elastic stack**. It is not a Kubernetes object, it cannot live in your namespace, and your CI deployer cannot apply it. That's inherent, not a gap in your setup — so (c) will need an owner on the `logging` side regardless of which option you pick. (a) and (b) are fully self-contained; (c) isn't, and no amount of arranging will make it so on a Basic licence.

<details>
<summary>What I touched (nothing)</summary>

Read-only throughout: `kubectl get/describe`, and read-only GETs against the ES and Kibana APIs via temporary port-forwards using the ECK-generated `elastic` user. No objects created, modified, or deleted; both port-forwards closed; no credential reproduced here or written to any file.
</details>

---

## PM rulings from this report (Dana, 12:00 PDT)

- **(a) and (b) proceed, self-contained in `loginid-takehome`.** The retention sweep persists its per-class results to the database it already reaches; `api-service` (the always-up pod) serves them as gauges on a separate `/metrics` port; one `ServiceMonitor` and one `PrometheusRule`, both labelled `release: prometheus`, both in our namespace; a NetworkPolicy ingress rule admitting `namespaceSelector: {kubernetes.io/metadata.name: monitoring}` on the metrics port only. Priya's three rules become Prometheus expressions over `time() - retention_sweep_last_success_timestamp_seconds`, per class. Severity `warning`, never `info`, for the skip-rate rule. No metric or label name may contain the substring `health` (Filebeat drops those lines silently).
- **The deployer Role needs `monitoring.coreos.com` (`prometheusrules`, `servicemonitors`).** That is a `deploy/rbac.yaml` change, bootstrap-applied, so it goes to Warren for the apply as before, and #62's preflight will now refuse to deploy until it is applied. Theo's PR states this in its body.
- **(c) does not fit on a Basic licence and is escalated to Warren**, not quietly closed: the two honest options are a dedicated audit data stream written by the application (then an index-level ES role that is genuinely scoped) or accepting namespace-wide read on the shared stream. Either needs a cluster-level ES role created by someone on the `logging` side. Recorded as an environment residual in the threat model until Warren rules.
- **LT-20 (log-sink retention) is now refinable**: the audit trail lives in the shared `filebeat-9.4.4` data stream, so retention is an ILM question on a shared object, and the story must say so.
