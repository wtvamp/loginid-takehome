# Observability + Never-Log Enforcement

Design-only. Covers logging, metrics, and tracing for both services, and — the harder half — how `handoff-04-secrets.md`'s never-log list gets enforced as a mechanism, not just stated as a policy.

## Logging

Structured logging (`slog`, Go stdlib — no third-party logging library needed for two services this size), JSON output, one line per event. Every log line that touches a request carries `sub` (caller identity), the endpoint, status, and a request id — never the request or response body wholesale. This is the pattern the never-log list requires (hand-off item 8: "full request bodies of `POST /auth` and `POST /identity`... both contain items above by construction") — the enforcement mechanism is that handlers log a small, explicit field set assembled by hand, never `log.Printf("%+v", req)` or equivalent reflection-based dumps of a request/response struct. A struct that contains a password field can't leak through a log statement that never receives the struct.

**Enforcement beyond "write it carefully":**
- A lint rule (a `golangci-lint` custom check, or a simple `go vet`-style grep in CI) that flags any log call passing a whole request/response struct, a `*http.Request` body, or any variable named/typed to match the never-log list's fields (`password`, `token`, `secret`, `dsn`, `client_secret`). Not foolproof — a determined developer can rename around it — but it catches the accidental case, which is the common one.
- Error-path discipline: wrapped errors are logged with `err.Error()` truncated/sanitized where the error type is known to carry sensitive detail (a DB driver error can echo the DSN; a vendor HTTP client error can echo the response body per hand-off item 4) — the wrapping layer strips those before the error reaches a log call, not the log call itself deciding case-by-case.

## Metrics

Standard RED metrics (rate, errors, duration) per endpoint, via Prometheus client library, scraped by whatever the lab cluster already runs (not stood up new for this take-home). Labels: method, route, status class — never a label with cardinality tied to user data (no `sub` or phone number as a metric label; that turns a metrics backend into an accidental PII store with no access control at all, which is a worse leak than a log line since metrics systems are typically more widely readable).

## Tracing

Not built out here — a two-service take-home with no running collector doesn't get more credible by naming OpenTelemetry. If ever added, it follows the same field discipline as logging.

## The never-log list, operationalized

Hand-off's nine items map to two enforcement surfaces, not nine separate mechanisms:

1. **Application-level:** the structured-logging discipline above (explicit field sets, the lint check, error-wrapping sanitization) covers items 1, 2, 5, 6, 7 (password, tokens, JWTs, client secrets, DSN) — these never enter a Go `log`/`slog` call in the first place.
2. **Proxy/ingress-level:** item 9 ("search query parameters containing a name or phone fragment... redact or hash before any access log, including proxy and ingress logs 04 controls") is the one item application code can't fully own, since ingress access logs are written by the ingress controller, not the service. Mechanism: an ingress-level log-format override that logs the request path with query string stripped (or a regex redaction on the `phone`/`name` query params specifically) rather than the default full-URL access log format. This is a one-time ingress ConfigMap change, not a per-request code path.
3. Item 3 (PII field values from `/identity` or `user_profile`) and item 4 (raw vendor error bodies) are the same discipline as #1 applied to the connector specifically — the connector logs "called vendor X, got status Y," never the response body.

## Audit-log access is a separate grant from Secret access

Per the hand-off's Kubernetes-specific requirements: the log stream carrying `sub`/scope/record-id/timestamp (the "fact of access" the never-log list still permits) is itself sensitive enough to reconstruct who looked up whom, so it gets its own RBAC — a distinct, narrower `Role` bound to an incident-review group, separate from any ServiceAccount's `Secret`-read grant (`./secrets-delivery.md`). Mechanism: if logs go to a cluster-local sink (e.g., Loki), that sink's read API gets its own RBAC/auth in front of it, not the same one guarding `kubectl get secrets`; if logs go to an external platform, the equivalent is a scoped read-only role in that platform, provisioned separately from any cluster credential.

## Retention-sweep observability (05's S5 requirement, final)

Per `../05-data-ops/pii-governance.md`: the retention sweep must emit, per data class per run, three metrics — rows examined, rows deleted, and **the age of the oldest surviving row in that class**. An alert fires when that age exceeds the class's stated retention window plus one sweep interval. This is the one metric in this document tied to a specific reasoning worth repeating: a sweep job that silently stops running produces no error and no symptom on its own — "rows deleted" can't tell a broken sweep from a legitimately empty one, since both report zero. Oldest-surviving-row age is the one number that keeps climbing when the job is dead and holds steady when it's healthy, which is why it's the alerting signal, not just a nice-to-have metric alongside the other two.

Mechanism: the sweep job (whatever invokes it — a CronJob against the lab cluster, per 05's mechanism-agnostic requirement) emits these as Prometheus metrics with a `class` label matching 05's actual retention classes — `source="direct"`, `source="idp_cache"`, and `idp_cache_orphan` for an orphaned `idp_cache` row (`../05-data-ops/pii-governance.md`), never a label carrying the PII itself, same discipline as the RED-metrics rule above. (An earlier draft of this section invented `vendor_token_cache`/`idp_identity_cache` as class names — those don't correspond to anything 05 tracks and described a connector token cache the S3 ruling never adopted; corrected here.) The alert rule is `oldest_surviving_row_age_seconds{class="X"} > (retention_window_seconds + sweep_interval_seconds)`, evaluated per class.

**Batched sweeps: only a drained result feeds the metric.** 05's `DeleteExpired` is batched (a bounded `maxRows` per call, `SweepResult.Drained` marking the last batch of a sweep) — `OldestSurvivingAt` is meaningful only on a `Drained == true` result, since mid-sweep the oldest deletable row is still present by construction and would read as stale on every batch but the last. The sweep job emits `oldest_surviving_row_age_seconds` only from drained results, never from an intermediate batch; the alert rule above already only ever sees what's published, so this is a constraint on what the job emits, not an extra filter the alert needs to apply. Getting this wrong means the alert fires on every sweep and gets muted, which is worse than not having it.

## Log-sink retention: the deletion-gap enforcement site

Application logs, metrics, and traces have no `source` column, no clock tied to a `user_profile` row, and no key a subject-deletion request could reach by cascade — a deleted profile's data can still exist in a log line indefinitely. 05's never-log list (`handoff-04-secrets.md`) and this document's enforcement mechanism (above) are the upstream control — if PII never enters a log, there's nothing in a log to expire — but that only holds for the fields on the list. What's left (chiefly the audit trail: `sub`, scope, opaque record id, timestamp — permitted by design, pseudonymous rather than PII-free) still needs a retention window: `pii-governance.md` § "Retention windows (S5)", the "Audit-log stream" row (clock: the entry's own timestamp; window: proposed, pending a legal ruling; disposal: **expiry at the sink**) names this document's log-sink retention policy as its enforcement site — the citation resolves in both directions. Not a deletion job 04 writes — a retention setting on infrastructure 04 already operates.

**Not the same object as the `deletion_log` table.** 05's `deletion_log` (rows written transactionally by `DeleteExpired`/`DeleteProfile`) is a separate thing from the audit-log stream above — it's PII-free by construction, carries no `RetentionClass`, and isn't in 05's retention table at all; nothing here applies to it, and no metric from a `SweepResult` does either. If a retention window is ever wanted for that table, it's a DAO contract change (05's to make), not a sink-retention config value (04's).

## What's deliberately not here

No SLOs/error-budget policy, no alerting-rule catalog beyond the retention-sweep alert above. A take-home needs the shape of an observability approach and a credible enforcement mechanism for the one hard requirement (never-log); it doesn't need a full SRE runbook.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from `handoff-04-secrets.md` v2's never-log list. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
