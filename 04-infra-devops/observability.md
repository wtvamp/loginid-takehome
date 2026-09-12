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

Mechanism: the sweep job (whatever invokes it — a CronJob against the lab cluster, per 05's mechanism-agnostic requirement) emits these as Prometheus metrics with a `class` label (one label value per retention class, e.g. `vendor_token_cache`, `idp_identity_cache` — never a label carrying the PII itself, same discipline as the RED-metrics rule above) rather than only a log line, since a log line nobody's alerting on has the same "silent failure" problem the requirement exists to solve. The alert rule is `oldest_surviving_row_age_seconds{class="X"} > (retention_window_seconds + sweep_interval_seconds)`, evaluated per class.

## What's deliberately not here

No SLOs/error-budget policy, no alerting-rule catalog, no log-retention-window design (that's 05's PII-retention territory for anything log-adjacent to PII, per `../05-data-ops/pii-governance.md` — this document only says logs must exclude PII values in the first place, not how long the remaining metadata is kept). A take-home needs the shape of an observability approach and a credible enforcement mechanism for the one hard requirement (never-log); it doesn't need a full SRE runbook.

---
*AI tooling note: drafted directly by Theo (Sonnet, this session) from `handoff-04-secrets.md` v2's never-log list. Bree's (capability-case) and Callum's (scope-cut) per-deliverable notes reviewed and folded in inline above, per PLAN.md §3.*
