# Hand-off 02 → 03: Auth scheme, validation point, and authorization requirements

Status: **v3** — updated after the cross-track consistency pass (`../decisions/cross-track-consistency.md`) resolved six findings owned by this track that touched this hand-off (F3, F5, F6, F8, F10, F-pag, F52); v2's S8-driven fixes are unchanged and folded in below as before. Receiver: Renata Cole, `../03-engineering-delivery/`. Written so 03 should be able to scaffold and build the middleware without re-deriving the reasoning in `api-auth-design.md`, `connector-security.md`, or the S3/S4/S8 decision records. Full reasoning, if wanted: `api-auth-design.md`, `decisions/search-authz-scoping.md`, `decisions/connector-token-lifecycle-redblue.md`.

## Token scheme

- **Grant type:** OAuth2 client-credentials. Machine-to-machine only; no end-user login flow for question 2's API.
- **Token format:** JWT, signed, asymmetric (RS256 or ES256 — pick one and use it consistently; RS256 is the safer default if your Go JWT library's ES256 support is less mature).
- **Claim set (minimum):** `sub` (calling client identity), `scope` (space-delimited, values below), `exp`, `iat`, `aud` (this API, specifically — reject tokens minted for another audience).
- **Literal values, filling the "pending 02's final scheme" placeholder in your Service boundaries config surface:** `AUTH_JWT_ISSUER = "https://auth.loginid-takehome.internal"`, `AUTH_JWT_AUDIENCE = "loginid-api-service"`. Both are configuration, not secrets — safe in a `ConfigMap` or committed default, per `handoff-04-secrets.md` row #3.
- **TTL:** 5–15 minutes. No refresh tokens — the client re-authenticates with its own client secret on expiry.
- **No denylist.** Revocation is handled by TTL shortness, not a stateful lookup. Don't build one; it's not missing, it's a deliberate trade-off already reasoned through in `api-auth-design.md`.
- **Verifier must pin the expected algorithm** (whichever of RS256/ES256 you choose) and reject a token whose header claims any other algorithm, including `alg: none` and a downgrade to a symmetric algorithm using the public key as an HMAC secret (RFC 8725 §3.1). Check your JWT library's default behavior — several popular libraries have shipped this vulnerability by not doing this by default.
- **Token-endpoint brute-force protection**, separate from the resource-API rate limits below: failed-grant rate limiting and progressive backoff per `client_id` and per source IP on the token-issuing endpoint itself, plus alerting on a sustained run of failed grants against one `client_id`. This protects the endpoint where a caller presents `client_id`/`client_secret`, which none of the resource-API limits below cover.
- **Bearer-token replay is an accepted risk, not a gap to close in implementation.** This token has the same possession-equals-authority property as the connector's vendor token (`connector-security.md`); no PoP/binding is expected or required here. Noted so it isn't mistaken for something to "fix" during implementation.

## Where it's validated

- **Middleware at the API edge**, in `internal/api/` per your published service boundary — every request to `cmd/api-service` passes through token validation before reaching a handler. `cmd/idp-connector` does not validate *this* token — but per `connector-security.md` §5 (added by S8 review), `idp-connector`'s own `/auth` and `/identity` handlers need the same JWT middleware pattern, with a **distinct audience** (`aud: idp-connector-service`, not `loginid-api-service`) and its own scope (`connector:identity-lookup`), restricted to internal callers (in this project's shape, `api-service` acting on a caller's behalf) — not the same token, not exposed to arbitrary external callers. Two audiences, one mechanism.
- **Key retrieval:** middleware verifies with the **public** key only (JWKS or a fetched public key — see `handoff-04-secrets.md` row #3). The private signing key never touches the verifying `api-service` Deployment; it lives with the token-issuing function, which is a mode of the same `api-service` image but runs as its **own Deployment** (`APP_MODE=issuer`, its own ServiceAccount), not the same Deployment doing both (corrected in `handoff-04-secrets.md` Assumption 2 and row #2, F13 — RBAC on a Secret's `get` verb doesn't stop a volume already mounted into a shared Deployment's containers, so the separation has to be a second Deployment, not RBAC alone). This is a second `APP_MODE` value for you to wire alongside your existing config surface.
- **Key rotation:** expect a `kid` claim; publish old and new public keys together during rotation overlap. Token TTL (5–15 min) bounds how long that overlap needs to last.

## Health endpoints (ruling, not open for re-litigation)

**`/healthz`/`/readyz` (or equivalent liveness/readiness probes) are unauthenticated on both binaries** — normal practice, no token check, no scope. Response body may carry an up/down boolean (optionally per-dependency, e.g. `"db": "ok"`) **plus one opaque build identifier (commit SHA / build id) for LT-34's deploy-freshness check** — nothing else version-shaped (no semver, no library version list, which maps to a public CVE lookup the way an opaque SHA doesn't), no connection details, no stack traces, no config values, nothing on `handoff-04-secrets.md`'s never-log list. Full reasoning: `api-auth-design.md`.

## Scope vocabulary (final, from S4 — `decisions/search-authz-scoping.md`)

| Scope | Authorizes | Does not authorize | Object-level check |
|---|---|---|---|
| `profile:read:own` | Retrieve a single record by id the caller already holds a legitimate reference to (e.g., a partner resolving a referred user) | Search/query by name or phone fragment; retrieval of a record outside the caller's referral relationship | `target.id` checked against the `caller_referral` mapping (see "Where `caller_referral` and the caller-role table live," below); out-of-set → reject, logged as a policy denial, distinct from an auth failure |
| `profile:read:any` | Retrieve a single record by id, no referral constraint | Search/query by fragment | Requires a `reason_code` from a closed enum (below) on every request, logged with the audit record |
| `profile:search` | Query by name/phone fragment across the table | — (this is the broadest scope; there is no broader one) | Result cap enforced; a query that would exceed it unconstrained (e.g., bare wildcard) is rejected outright, not truncated |

`reason_code` enum for `profile:read:any` (closed, versioned — adding a value is a documented change, not a runtime config toggle): `support_ticket`, `fraud_review`, `kyc_reverification`, `legal_hold`.

**Object-level policy hook implementation shape:** a function `authorize(sub, scope, target) -> allow|deny`, called after scope validation, before the handler executes the query/retrieve. This is a second check, not a replacement for the scope check — a valid `profile:read:own` token can still fail `authorize()` if the target isn't in the caller's referral set. **Confirmed placement (per your reply): an explicit call inside each handler right after parsing the request and before the DAO call, not a second middleware** — correct, since `authorize()` needs the parsed target id, which a pre-routing middleware doesn't have. **Enforcement site for the `profile:search` result cap: the same in-handler call site, checked before the query is built** — a query that would exceed the cap unconstrained is rejected there, not truncated after the DAO returns rows.

**Where `caller_referral` and the caller-role table live (resolves F8 — this data was previously asserted as "owned by `../05-data-ops/`" without 05 ever having received that assignment; 05's schema has no such table and shouldn't grow one for this).** Both live in the token-issuing authorization server's own datastore (the same one that holds the hashed `client_secret`s per `handoff-04-secrets.md` row #4), not in `05-data-ops`'s `user_profile`/`user_credential` schema. This is authorization-provisioning data — who a client credential is allowed to act on behalf of — not subject data, and keeping it out of 05's schema also keeps it out of that schema's PII-governance/retention rules, which don't apply to it. 05 owns nothing here; this hand-off's earlier wording was wrong to say otherwise.

**Response shape and masking (resolves F6 — 01's product requirement had no mechanism to bind to).** `profile:search` responses carry masked PII fields by default (e.g., a phone shown as `***-***-1234`, an address reduced to `locality`/`region` only) — unmasked, full-field detail is obtained only by a follow-up call under `profile:read:own` or `profile:read:any` against the specific record id returned by the search, which is its own separately-audited call per the audit-log section below. This uses scopes that already exist rather than inventing a fourth "expand" concept: masking is a `profile:search`-response property, not a per-call flag, and "tighter than search" is satisfied structurally because `read:own`/`read:any` are already narrower-provisioned scopes than `search`.

**Reject an empty search body before the DAO (resolves F10, first half).** A `profile:search` request with no `name` or `phone` filter at all is rejected by the handler with a 400 before it ever reaches the DAO — the DAO's own "an all-nil query is legal and returns every profile" behavior (`../05-data-ops/multi-db-strategy.md`) is a DAO-layer property this API must not expose directly. This is a shape-based rejection, distinct from and in addition to the count-based result-cap rejection above.

## Pagination, rate limits, decomposition resistance (requirements — numbers are fixed, mechanism is yours)

- **Opaque cursor at the API boundary** (resolves F-pag — the earlier "cursor-based pagination only, no offset-based paging" was wrong to imply offset itself is the enumeration risk: a keyset cursor permits the same page-by-page walk, just statefully; the actual control is the cumulative counter below, and 05's DAO contract is `Offset int`, not keyset). The API-facing cursor may encode the DAO's offset directly — callers cannot construct or walk one by hand, which is the property the boundary exists to give you, and the DAO's offset is never caller-visible. **Enforcement site: the cursor-encoding/decoding step in the handler layer**, opaque to the caller regardless of what it encodes underneath. Default page size 20, hard ceiling 50 (05's DAO ceiling is 10,000 — this API's ceiling is the tighter, caller-facing one; don't expose the DAO's).
- **Per-client-credential rate limits:** 60 req/min for `read:own`/`read:any`; 10 req/min for `profile:search`. **Enforcement site: rate-limiting middleware in `internal/api/`, keyed per client credential (`sub` claim).**
- **Cumulative distinct-record-touch counter per caller per rolling 24h window**, tracked independently of per-minute rate — this is the control that actually resists decomposition-style enumeration (not the pagination scheme, per the F-pag correction above). **Applies to `profile:read:any` as well as `profile:search`, unconditionally on whether a `reason_code` is present** — a valid, logged `reason_code` satisfies the per-call `authorize()` policy check but does not exempt the caller from this cumulative cap (S8 review fix: the original wording made this counter a no-op for `read:any`, since that scope requires a `reason_code` on every call by design). **Enforcement site: a counting store (Redis, in-process with periodic flush, whatever fits your stack) incremented once per distinct record id returned, checked before the response is sent** — the mechanism is your call; the requirement is that it exists and applies to both scopes.

**Ruling on threshold-crossing behavior (clarifying an ambiguity Tobias Lindqvist caught in LT-39 refinement — this was genuinely unclear as written, not a judgment call I'd already made and failed to state):** crossing the cumulative cap **hard-denies (429) further requests on that scope for the remainder of the rolling window**, in addition to alerting — not alert-only, not "log it and keep serving." An alert-only reading would make this counter a detection mechanism rather than the control S4/S8 built it to be, and would recreate exactly the "attribution stands in for volume control" gap the counter exists to close (Ingrid's S4 objection 4) — a caller who's crossed the cap and keeps getting served has, in practice, no cap. Both signals fire together: the 429 to the caller, and the alert to whoever monitors it, because a threshold that's silently enforced with no alert is just as bad operationally as one that alerts with no enforcement.
- Sustained near-limit usage on any of the above should alert, not just silently throttle. **Enforcement site: the same rate-limiting middleware and counting store above, not a separate mechanism.**

## Audit-log field list (every search/retrieve call)

Log these fields, and only these — see `handoff-04-secrets.md`'s never-log list for what must never appear here:

- `sub` (caller identity)
- `scope` used
- record id(s) touched (opaque ids, not the PII values)
- `reason_code`, when the scope is `profile:read:any`
- policy decision (`allow` / `deny`, and if `deny`, whether it was a scope failure or an `authorize()` policy denial — these are distinct signals for detection)
- timestamp
- request outcome (success/failure/rate-limited)

Never in this log: PII field values, full JWTs, full request/response bodies. Per `handoff-04-secrets.md`'s new requirement, this log stream itself needs read access distinct from and narrower than whatever can read Secrets — that's 04's mechanism, but don't let the log ship without asking whether that separation exists.

## What's not yours to decide (already decided upstream, don't re-derive)

- **Grant type, token format, and claim set** — decided in `api-auth-design.md`; not open for re-litigation during implementation.
- **Scope vocabulary and provisioning bars** — decided in S4 (`decisions/search-authz-scoping.md`); if a real implementation constraint makes one of these unworkable, flag it back to me rather than adjusting it unilaterally.
- **The `DB_DSN_FILE` convention** — per `handoff-04-secrets.md`'s S7 revision, this track is asking you to add a file-based alternative to `DB_DSN` (env var becomes a transitional fallback, not a permanent equal option), because a DSN's embedded password is exposed by `kubectl describe`/crash dumps/child-process environment the same as any other secret in that inventory. This is a request into your config surface, not a unilateral change on my part — you decide the exact mechanism (a `_FILE` suffix convention or otherwise), I'm asserting the requirement.
- **Password/client-secret hashing** — Argon2id, decided in `threat-model.md` Asset 1; not this hand-off's concern beyond noting it exists.
- **PII retention/caching duration for anything the API returns** — `../05-data-ops/CLAUDE.md`'s call, not this track's or yours.
- **Whether `read:any` ever gets issued to a real caller** — per S4, the scope stays defined but unissued unless a caller clears the provisioning bar (caller-role table entry, named approver). Don't wire a default grant "just in case."

## Open question back to you

Nothing blocking. One thing worth a quick confirm when convenient: does your `internal/api/` middleware layer have a natural place to run the object-level `authorize()` check as a distinct step from scope validation (e.g., a second middleware or an explicit call inside each handler before the DAO call), or would you rather this track propose a specific Go interface shape for it? Either is fine — say which you'd find more useful and I'll adjust.

---
Model: sonnet (Marcus Ilori, lead — compiled directly from S4's ruling, no new debate). Turns consumed: 0 hire turns (synthesis of already-ruled decisions per `PLAN.md` S6).
