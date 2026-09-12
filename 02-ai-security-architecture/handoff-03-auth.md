# Hand-off 02 → 03: Auth scheme, validation point, and authorization requirements

Status: **v1**. Receiver: Renata Cole, `../03-engineering-delivery/`. Written to the project's hand-off standard: 03 should be able to scaffold and build the middleware without re-deriving the reasoning in `api-auth-design.md`, `connector-security.md`, or the S3/S4 decision records. Full reasoning, if wanted: `api-auth-design.md`, `decisions/search-authz-scoping.md`, `decisions/connector-token-lifecycle-redblue.md`.

## Token scheme

- **Grant type:** OAuth2 client-credentials. Machine-to-machine only; no end-user login flow for question 2's API.
- **Token format:** JWT, signed, asymmetric (RS256 or ES256 — pick one and use it consistently; RS256 is the safer default if your Go JWT library's ES256 support is less mature).
- **Claim set (minimum):** `sub` (calling client identity), `scope` (space-delimited, values below), `exp`, `iat`, `aud` (this API, specifically — reject tokens minted for another audience).
- **Literal values, filling the "pending 02's final scheme" placeholder in your Service boundaries config surface:** `AUTH_JWT_ISSUER = "https://auth.loginid-takehome.internal"`, `AUTH_JWT_AUDIENCE = "loginid-api-service"`. Both are configuration, not secrets — safe in a `ConfigMap` or committed default, per `handoff-04-secrets.md` row #3. If `cmd/idp-connector` ever needs its own audience value (it doesn't consume this token scheme today — see "Where it's validated" below), that would be a second, distinct `aud`, not a shared one; not needed for this submission.
- **TTL:** 5–15 minutes. No refresh tokens — the client re-authenticates with its own client secret on expiry.
- **No denylist.** Revocation is handled by TTL shortness, not a stateful lookup. Don't build one; it's not missing, it's a deliberate trade-off already reasoned through in `api-auth-design.md`.

## Where it's validated

- **Middleware at the API edge**, in `internal/api/` per your published service boundary — every request to `cmd/api-service` passes through token validation before reaching a handler. `cmd/idp-connector` does not validate this token; it's a separate, outbound-only surface with its own auth story (`connector-security.md`), not a consumer of this scheme.
- **Key retrieval:** middleware verifies with the **public** key only (JWKS or a fetched public key — see `handoff-04-secrets.md` row #3). The private signing key never touches `api-service`; it lives with the token-issuing function, which is a mode of `api-service` (not a separate binary — see `handoff-04-secrets.md` Assumption 2), scoped by RBAC so the verification code path and the issuing code path don't share Secret access even though they're the same binary.
- **Key rotation:** expect a `kid` claim; publish old and new public keys together during rotation overlap. Token TTL (5–15 min) bounds how long that overlap needs to last.

## Scope vocabulary (final, from S4 — `decisions/search-authz-scoping.md`)

| Scope | Authorizes | Does not authorize | Object-level check |
|---|---|---|---|
| `profile:read:own` | Retrieve a single record by id the caller already holds a legitimate reference to (e.g., a partner resolving a referred user) | Search/query by name or phone fragment; retrieval of a record outside the caller's referral relationship | `target.id` checked against `caller_referral` mapping (data owned by `../05-data-ops/`); out-of-set → reject, logged as a policy denial, distinct from an auth failure |
| `profile:read:any` | Retrieve a single record by id, no referral constraint | Search/query by fragment | Requires a `reason_code` from a closed enum (below) on every request, logged with the audit record |
| `profile:search` | Query by name/phone fragment across the table | — (this is the broadest scope; there is no broader one) | Result cap enforced; a query that would exceed it unconstrained (e.g., bare wildcard) is rejected outright, not truncated |

`reason_code` enum for `profile:read:any` (closed, versioned — adding a value is a documented change, not a runtime config toggle): `support_ticket`, `fraud_review`, `kyc_reverification`, `legal_hold`.

**Object-level policy hook implementation shape:** a function `authorize(sub, scope, target) -> allow|deny`, called after scope validation, before the handler executes the query/retrieve. This is a second check, not a replacement for the scope check — a valid `profile:read:own` token can still fail `authorize()` if the target isn't in the caller's referral set.

## Pagination, rate limits, decomposition resistance (requirements — numbers are fixed, mechanism is yours)

- **Cursor-based pagination only** — no offset-based paging. Default page size 20, hard ceiling 50.
- **Per-client-credential rate limits:** 60 req/min for `read:own`/`read:any`; 10 req/min for `profile:search`.
- **Cumulative distinct-record-touch counter per caller per rolling 24h window**, tracked independently of per-minute rate — this is the control that catches slow, under-the-cap enumeration (decomposition) that per-request and per-minute limits alone miss. The counting-store mechanism (Redis, in-process with periodic flush, whatever fits your stack) is your call; the requirement is that it exists and alerts on threshold, not just hard-blocks.
- Sustained near-limit usage on any of the above should alert, not just silently throttle.

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
