# Observability + Never-Log Enforcement

Design-only. Covers logging, metrics, and tracing for both services, and — the harder half — how `handoff-04-secrets.md`'s never-log list gets enforced as a mechanism, not just stated as a policy.

## Logging

Structured logging (`slog`, Go stdlib — no third-party logging library needed for two services this size), JSON output, one line per event. Every log line that touches a request carries `sub` (caller identity), the endpoint, status, and a request id — never the request or response body wholesale. This is the pattern the never-log list requires (hand-off item 8: "full request bodies of `POST /auth` and `POST /identity`... both contain items above by construction") — the enforcement mechanism is that handlers log a small, explicit field set assembled by hand, never `log.Printf("%+v", req)` or equivalent reflection-based dumps of a request/response struct. A struct that contains a password field can't leak through a log statement that never receives the struct.

**Enforcement beyond "write it carefully":**
- A lint rule (a `golangci-lint` custom check, or a simple `go vet`-style grep in CI) that flags any log call passing a whole request/response struct, a `*http.Request` body, or any variable named/typed to match the never-log list's fields (`password`, `token`, `secret`, `dsn`, `client_secret`). Not foolproof — a determined developer can rename around it — but it catches the accidental case, which is the common one.
- Error-path discipline: wrapped errors are logged with `err.Error()` truncated/sanitized where the error type is known to carry sensitive detail (a DB driver error can echo the DSN; a vendor HTTP client error can echo the response body per hand-off item 4) — the wrapping layer strips those before the error reaches a log call, not the log call itself deciding case-by-case.

## Filebeat's `drop_event` regex over-matched "health" — applied by platform, 2026-09-13 12:18 PDT

Amber's observability inventory (LT-49) found Filebeat's `drop_event` filter dropping any log line whose `message` matches `(kube-probe|health|/healthz|/readyz)` — a bare substring match on "health" anywhere in the line, not anchored to an actual health-check request. Marcus (02) escalated this as a required fix (`threat-model.md` Assumption 17): a silently dropped line is indistinguishable from an event that never happened, the inverse of this project's own never-log discipline (which governs what must never be *written*, not evidence disappearing after the fact with no trace).

**This track did not apply the fix — same class of action as the ES/Kibana question this story escalated.** The regex lives in `ConfigMap/filebeat-filebeat-daemonset-config` in namespace `logging`, a shared object every tenant's logs flow through (Amber's inventory, section 1). The corrected regex below was proposed here, escalated by the PM to Warren, and **applied by Amber on Warren's authorization**. Confirmed applied from the cluster itself, not the ConfigMap edit alone: `DaemonSet/filebeat-filebeat` showed 5 desired / 5 updated / 5 ready, all five pods recreated at 2026-09-13 19:18 UTC (12:18 PDT), after the ConfigMap edit — the PM verified the rollout directly.

- **Was:** `(kube-probe|health|/healthz|/readyz)` — over-broad; "health" alone matched any line containing those five letters anywhere (a field name, a vendor string, a coincidence), silently.
- **Now:** anchored to the actual shapes a health-check line can take, not a bare word — `(kube-probe/|"path":"/healthz"|"path":"/readyz"|(GET|HEAD) /healthz |(GET|HEAD) /readyz )`. Covers the `kube-probe` User-Agent literally (with its trailing `/` so it can't match an unrelated word containing "kube-probe" as a prefix of something else), a structured-log `path` field holding exactly `/healthz`/`/readyz` (quoted, so a substring inside a longer path like `/healthzzzz` or a URL query value doesn't match), and the plaintext access-log request-line shape (method plus the literal path token, space-delimited on both sides so it can't match a path merely containing those characters as a substring).

**Our own mitigation stands regardless of when/whether the shared regex is fixed:** no metric or label name emitted by this project may contain the substring `health`, since a metric that never reaches the sink is exactly as invisible as an audit-log line that never reaches it, whatever the eventual regex fix looks like. Applies to every metric introduced from LT-49 onward (`retention_sweep_last_success_timestamp_seconds`, `oldest_surviving_row_age_seconds`, `retention_sweep_skipped_total`, `rows_examined`, `rows_deleted` — none do).

## Metrics

Standard RED metrics (rate, errors, duration) per endpoint, via Prometheus client library, scraped by whatever the lab cluster already runs (not stood up new for this take-home). Labels: method, route, status class — never a label with cardinality tied to user data (no `sub` or phone number as a metric label; that turns a metrics backend into an accidental PII store with no access control at all, which is a worse leak than a log line since metrics systems are typically more widely readable).

**A `ServiceMonitor` selects `Service` objects by label — not by the `Service`'s own `selector` (which pods it routes to).** Found live during LT-49's rollout: `deploy/api-service.yaml`'s Service had `selector: {app: api-service}` (routing pods correctly, ports/Endpoints all correct) but carried no `labels:` of its own, so `loginid-takehome-api-service`'s `spec.selector.matchLabels: {app: api-service}` matched zero `Service` objects — `/api/v1/targets` showed zero targets for this namespace despite a healthy Endpoints object with `metrics:9101` present. **The `Service` must carry `app=<name>` as its own `metadata.labels`, in addition to whatever `spec.selector` it uses to route traffic — the two are unrelated fields serving different consumers (Prometheus reads the former, kube-proxy the latter).**

## Tracing

Not built out here — a two-service take-home with no running collector doesn't get more credible by naming OpenTelemetry. If ever added, it follows the same field discipline as logging.

## The never-log list, operationalized

**Corrected (LT-49, per Desmond's claims check):** this section previously said "nine items" and enumerated only 1–9 — it predates the cross-track consistency pass (F50) that added items 10 (driver-error `DETAIL` text) and 11 (username) to `handoff-04-secrets.md`'s list, and was never back-propagated. All eleven items map to the same two enforcement surfaces below, not nine:

1. **Application-level:** the structured-logging discipline above (explicit field sets, the lint check, error-wrapping sanitization) covers items 1, 2, 5, 6, 7 (password, tokens, JWTs, client secrets, DSN) — these never enter a Go `log`/`slog` call in the first place.
2. **Proxy/ingress-level:** item 9 ("search query parameters containing a name or phone fragment... redact or hash before any access log, including proxy and ingress logs 04 controls") was written assuming search takes `name`/`phone` as query-string parameters. **Corrected (LT-49, verified against the real implementation and the real cluster, not assumed):** `internal/app/verifier_router.go` routes search as `POST /profiles/search` with `name`/`phone` in a JSON request body (`internal/api/handlers.go`'s `NewSearchHandler`, `json.NewDecoder(r.Body)`) — there is no query string carrying this data at all. `GET /profiles/{id}` takes an opaque record id as a path segment, not name/phone, so it's not a concern either. Checked the real ingress-nginx ConfigMap directly (`kubectl get configmap -n ingress-nginx ingress-nginx-controller -o yaml`): no custom `log-format-upstream` override exists — the controller uses its default access-log format (`$request` = method, path, protocol only), which never includes the request body regardless. **Net: under the actual implementation and the cluster's current, verified configuration, no query-string PII reaches an ingress access log today** — there is nothing to redact, not because redaction was applied, but because the risk this item names doesn't materialize for this endpoint's actual request shape. This is a live tripwire, not a closed question forever: if a future change ever adds a query-string-based search variant (a GET alternative, a debug endpoint, a redirect that echoes search terms in a URL), the redaction mechanism this item originally called for would then be genuinely needed and does not yet exist — flag that specifically at review time if it comes up, rather than assuming this finding still holds.
3. Item 3 (PII field values from `/identity` or `user_profile`) and item 4 (raw vendor error bodies) are the same discipline as #1 applied to the connector specifically — the connector logs "called vendor X, got status Y," never the response body.
4. Item 10 (database driver error `DETAIL`/`Key (...)=(...)` text) is the error-path discipline bullet above, applied to 03's own error-translation layer (`error-semantics.md`) specifically: SQLSTATE/result code and constraint name only reach a log call, the sanitization happening before the error is wrapped, not decided case-by-case at the log call itself — application-level, same surface as #1.
5. Item 11 (username) is the same discipline as item 3: a hash or opaque identifier for correlation, never the raw value, application-level.

## Audit-log access is a separate grant from Secret access

Per the hand-off's Kubernetes-specific requirements: the log stream carrying `sub`/scope/record-id/timestamp (the "fact of access" the never-log list still permits) is itself sensitive enough to reconstruct who looked up whom, so it gets its own RBAC — a distinct, narrower `Role` bound to an incident-review group, separate from any ServiceAccount's `Secret`-read grant (`./secrets-delivery.md`). Mechanism: if logs go to a cluster-local sink (e.g., Loki), that sink's read API gets its own RBAC/auth in front of it, not the same one guarding `kubectl get secrets`; if logs go to an external platform, the equivalent is a scoped read-only role in that platform, provisioned separately from any cluster credential.

## Retention-sweep observability (05's S5 requirement, final)

Per `../05-data-ops/pii-governance.md`: the retention sweep must emit, per data class per run, three metrics — rows examined, rows deleted, and **the age of the oldest surviving row in that class**. An alert fires when that age exceeds the class's stated retention window plus one sweep interval. This is the one metric in this document tied to a specific reasoning worth repeating: a sweep job that silently stops running produces no error and no symptom on its own — "rows deleted" can't tell a broken sweep from a legitimately empty one, since both report zero. Oldest-surviving-row age is the one number that keeps climbing when the job is dead and holds steady when it's healthy, which is why it's the alerting signal, not just a nice-to-have metric alongside the other two.

Mechanism: the sweep job (a `CronJob`, `deploy/manifests.yaml`, schedule `17 3 * * *` — I = 24h = `sweep_interval_seconds` = 86400) emits these as Prometheus metrics with a `class` label matching 05's actual retention classes — `source="direct"`, `source="idp_cache"`, and `idp_cache_orphan` for an orphaned `idp_cache` row (`../05-data-ops/pii-governance.md`), never a label carrying the PII itself, same discipline as the RED-metrics rule above. (An earlier draft of this section invented `vendor_token_cache`/`idp_identity_cache` as class names — those don't correspond to anything 05 tracks and described a connector token cache the S3 ruling never adopted; corrected here.)

**Alert rule, per Priya Nandakumar's (05) LT-49 ruling — three rules, none suppressing another** (superseding this section's earlier single-formula design, `refinement/LT-49.md`): a single "age exceeds window + interval" formula can't tell a `concurrencyPolicy: Forbid`-skipped run from a dead sweep, and suppressing on the skip signal is actively dangerous — a chronically-overrunning sweep would emit a skip signal every interval and permanently silence the one alert meant to catch it. Let **W** = the class's retention window, **I** = `sweep_interval_seconds` (86400):

1. **`RetentionSweepNotReporting`** (per class): fires when no fresh `oldest_surviving_row_age_seconds` sample for that class in more than **2I**. A single skip produces a gap of exactly 2I and does not fire (the grace for a skip); two consecutive skips (3I) does fire, as does a genuinely dead sweep. Counts missed samples rather than trusting an explanation — does not consume the skip-visibility signal at all.
2. **`RetentionWindowExceeded`** (per class): fires when `oldest_surviving_row_age_seconds{class} > W + 2I`. The first `I` is legitimate lag (a row becomes eligible just after one sweep and waits a full interval for the next); the second absorbs one skipped run. Assumes sweep duration `D < I` — if `D` approaches `I`, rule 3 is the real signal.
3. **`RetentionSweepSkipRate`** (low severity, early warning): fires when skipped runs exceed ~25% over a rolling day. This is where LT-44's `concurrencyPolicy: Forbid` skip-visibility signal (the `JobAlreadyActive` event) is actually consumed — as evidence in its own right, never as a suppression term on rules 1 or 2.

A sweep that runs but never drains (hitting `maxRows` every batch) emits no valid sample and correctly trips rule 1 — that is the intended behavior, not a false positive to "fix" by publishing non-drained values (see the batching note below).

**Batched sweeps: only a drained result feeds the metric.** 05's `DeleteExpired` is batched (a bounded `maxRows` per call, `SweepResult.Drained` marking the last batch of a sweep) — `OldestSurvivingAt` is meaningful only on a `Drained == true` result, since mid-sweep the oldest deletable row is still present by construction and would read as stale on every batch but the last. The sweep job emits `oldest_surviving_row_age_seconds` only from drained results, never from an intermediate batch; the alert rule above already only ever sees what's published, so this is a constraint on what the job emits, not an extra filter the alert needs to apply. Getting this wrong means the alert fires on every sweep and gets muted, which is worse than not having it.

## Log-sink retention: the deletion-gap, and the ILM request (LT-20, corrected)

Application logs, metrics, and traces have no `source` column, no clock tied to a `user_profile` row, and no key a subject-deletion request could reach by cascade — a deleted profile's data can still exist in a log line indefinitely. 05's never-log list (`handoff-04-secrets.md`) and this document's enforcement mechanism (above) are the upstream control — if PII never enters a log, there's nothing in a log to expire — but that only holds for the fields on the list. What's left (chiefly the audit trail: `sub`, scope, opaque record id, timestamp — permitted by design, pseudonymous rather than PII-free) still needs a retention window.

**Corrected (LT-20, per `refinement/LT-20.md`): this is not a retention setting 04 configures.** The audit trail lands in Filebeat's shared `filebeat-9.4.4` Elasticsearch data stream (Amber's observability inventory) — infrastructure `logging`'s operators own, not this track, and not a Kubernetes object any RBAC grant in this namespace reaches. This section states the requirement and names the owner; it is not applied configuration, and this story does not close by proxy the moment a mechanism that happens to touch the same stream (LT-49's dedicated-audit-stream option, if ever adopted) exists — each is independently confirmed.

**The ruled number (Priya, 05 — `pii-governance.md`): ≥ 90 days, a floor, not a ceiling.** Read that distinction literally, not as a caveat: every other number in `pii-governance.md` is a *maximum* driven by minimization (delete PII no longer than necessary); this one is a *minimum* driven by incident review (keep audit evidence no less than necessary). The audit stream carries no PII payload under the never-log discipline, and its subject identifiers (profile UUIDs, client_ids) stop being linkable to a person once the profile they name is deleted — so nothing pushes this number down; the only question was how far up. It's short (90 days, not a year) specifically because the durable proof-of-deletion lives in the `deletion_log` **table**, not this stream — the stream is corroborating detail for the ordinary incident-review horizon, not primary evidence. Had the table not existed, the floor would be materially longer.

**The ILM policy object, as a request, not an applied change** — this is the exact document to hand to whoever administers the shared Elastic stack (`logging` namespace; Warren today, on this cluster):

```json
{
  "policy": {
    "phases": {
      "hot": {
        "min_age": "0ms",
        "actions": { "rollover": { "max_age": "1d" } }
      },
      "delete": {
        "min_age": "90d",
        "actions": { "delete": {} }
      }
    }
  }
}
```

**Why `min_age: "90d"` on the `delete` phase, not a `max_age`/rollover trim:** this is the one line the whole request lives or dies on. `min_age` on `delete` means Elasticsearch *may not* move a document into deletion before 90 days have passed since rollover — a **floor**. A `max_age`-based trim (or a `delete` phase with a *shorter* `min_age`) reads similarly in English but is the *opposite* mechanism — a ceiling that would remove audit evidence this ruling exists specifically to preserve. Get this line wrong and the fix looks identical to the request until the first incident review comes up short.

**Stream-level minimum, not a project-specific one — stated as a hard platform fact, not a preference.** The audit events share `filebeat-9.4.4` with every other tenant's logs, and the ES license is **Basic** — document-level security (which would let a policy scope by `kubernetes.namespace: loginid-takehome`) is a Platinum/Enterprise feature (confirmed for read-role scoping in Amber's inventory; extrapolated, not separately confirmed, for ILM — flagged as the first thing to verify with the stream's owner, not assumed). The request above is therefore necessarily on the **whole stream's** minimum: it may retain longer for other tenants' own reasons, it must not retain less than 90 days for anyone, ours included. If a future architecture (LT-49's Option A — a dedicated `loginid-takehome-audit-*` stream written by `api-service` itself) ever lands, this same policy object would apply narrowly to that stream by name instead — but that's a *separate* request to the same owner, confirmed independently, not something this story inherits automatically the moment that stream exists (Tobias's objection, `refinement/LT-20.md`).

**Owner: whoever administers the shared Elastic stack in `logging` — Warren, on this cluster, today.** This section is the artifact handed to that owner; applying it, or ruling that the stream's existing policy (if any) already satisfies the floor, is their decision to make and record, not a config value this track can set or verify was set.

**Three revisit triggers (Priya's ruling), each stated with what it means operationally:**

1. **`deletion_log`'s own window is ever ruled shorter than 90 days.** The 90-day floor here depends on that table being the durable record; if the table becomes more perishable than this stream, the stream becomes primary evidence and this floor must rise to cover what the table no longer does.
2. **Any contractual or regulatory retention commitment appears** (a PCI-style one-year obligation, a customer contract). The floor stops being an engineering judgment call and becomes a minimum someone is legally accountable for — revisit the number, not just the mechanism.
3. **The never-log discipline breaks** — the dangerous one. If PII ever reaches this stream, it acquires a *minimum* from incident review and a *maximum* from storage/data-minimization law at the same time — two constraints that can conflict with no correct configuration available. This is why the never-log list (above) is load-bearing for this ruling, not merely good hygiene elsewhere in this document: it's the thing keeping this a one-sided constraint at all.

**Not the same object as the `deletion_log` table.** 05's `deletion_log` (rows written transactionally by `DeleteExpired`/`DeleteProfile`) is a separate thing from the audit-log stream above — it's PII-free by construction, carries no `RetentionClass`, and isn't in 05's retention table at all; nothing here applies to it, and no metric from a `SweepResult` does either. If a retention window is ever wanted for that table, it's a DAO contract change (05's to make), not a sink-retention config value (04's).

## What's deliberately not here

No SLOs/error-budget policy, no alerting-rule catalog beyond the retention-sweep alert above. A take-home needs the shape of an observability approach and a credible enforcement mechanism for the one hard requirement (never-log); it doesn't need a full SRE runbook.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from `handoff-04-secrets.md` v2's never-log list. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
