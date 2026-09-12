# API Authentication & Authorization Design — Question 2

Owner: Marcus Ilori (`02-ai-security-architecture`). Covers the RESTful API service to search/retrieve `user_profile` data (assignment question 2, including its explicit "security API authentication" requirement). Implementation target: `../03-engineering-delivery/CLAUDE.md`. Consumes the schema from `../05-data-ops/CLAUDE.md`.

## What "security API authentication" is scoped to mean here

The assignment names it without specifying a mechanism, so this is a design decision, not a given — stated with reasoning per this project's working principle.

Three realistic candidates and why I'm choosing among them:

| Option | Fit here | Verdict |
|---|---|---|
| Static API keys | Simple, but no expiry semantics, no scoping without inventing one, easy to leak into logs/URLs, no standard revocation flow | Rejected as the primary mechanism — acceptable only as a secondary, coarse-grained "which client system is this" identifier, never as sole authn/authz. |
| mTLS | Strong, but assumes both parties (this API and its callers) can manage client certs — reasonable for a closed set of internal service callers, awkward for anything resembling a broader consumer base | Recommended as a **transport-layer supplement** for service-to-service callers where the deploying org already runs a cert infrastructure; not the sole control. |
| OAuth2 client-credentials grant → short-lived JWT bearer token | Standard, expresses scopes natively, well-supported Go tooling, matches how the connector in question 3 already authenticates to us conceptually (token-based), and composes cleanly with per-caller authorization | **Chosen primary mechanism.** |

**Decision: OAuth2 client-credentials grant issuing short-lived JWT bearer access tokens, with mTLS as an optional additional transport control for known service-to-service callers.**

This is a machine-to-machine API (question 2 doesn't describe an end-user-facing login flow — it describes callers searching/retrieving profile data), so client-credentials is the correct OAuth2 grant type, not authorization-code (which is for delegated end-user consent) or password grant (deprecated, and inappropriate for a service caller).

## Token shape and lifecycle

- **Format**: JWT, signed (not encrypted — it carries no secret, only claims), asymmetric signing (RS256/ES256) so the API can verify tokens without holding the signing key, which itself lives with a token-issuing authorization server component.
- **Claims, minimum set**: `sub` (calling client/service identity), `scope` (space-delimited authorization scopes, see below), `exp` (short — 5–15 minutes), `iat`, `aud` (this API specifically, to prevent a token minted for another service being replayed here).
- **No refresh tokens** for a client-credentials grant — the client re-authenticates with its own client secret when the access token expires. This bounds the blast radius of a leaked access token to its short TTL.
- **Revocation**: because JWTs are self-contained, true revocation requires either a short enough TTL that revocation is unnecessary in practice (the approach here) or a token-denylist check, which reintroduces a stateful lookup and defeats part of the point of JWTs. Given the 5–15 minute TTL, denylist is not required for this design; flagged here so a reviewer sees the trade-off was considered, not missed.

## Authorization model: scopes, not just "authenticated"

The threat model (`threat-model.md`, Asset 2) already names the core risk: an authenticated-but-unscoped search/retrieve endpoint is a PII enumeration engine. Authentication answers "who is calling"; it does not answer "what may they see." This API needs both.

Finalized by the S4 adversarial pair (`decisions/search-authz-scoping.md`; Helena Marsh, Proposer; Ingrid Solano, Skeptic) — this section supersedes the earlier sketch.

### Scope vocabulary

- **`profile:read:own`** — retrieve a single record by an id the caller already holds a legitimate reference to (e.g., a partner resolving a user it referred). Enforced against a `caller_referral` mapping (below), not by scope alone. Does not authorize search/query by name or phone fragment.
- **`profile:read:any`** — retrieve a single record by id, no referral constraint. Not a default grant: provisioning requires an entry in an explicit caller-role table (service name, business justification, and specifically *why* `read:own`'s referral constraint is insufficient for that caller's function), gated the same way `profile:search` is below. If no caller meets that bar, the scope stays defined but unissued.
- **`profile:search`** — query by name/phone fragment across the table; the PII-enumeration-engine risk the threat model names. Its own gated capability, not "`read:any` plus search."

### `profile:search` provisioning bar (all four required)

1. Named service identity — never a shared client credential across a team.
2. Documented business function on file before provisioning, not granted speculatively.
3. Change-controlled approval with a named human approver, logged.
4. Tighter rate/result limits than any other scope (below).

### Object-level policy hook

A per-scope policy function `authorize(sub, scope, target)` runs after the scope check, not instead of it — directly targeting OWASP API1:2023 (Broken Object Level Authorization):

- **`read:own`**: checks `target.id` against the `caller_referral` mapping. Scope-valid but outside the caller's referral set → reject, logged as a policy denial (distinct from an auth failure, for detection). `caller_referral` entries are data owned by `../05-data-ops/CLAUDE.md`, written only through the same partner-onboarding workflow as any other provisioning decision (named approver, logged) — no ad hoc inserts.
- **`read:any`**: requires a `reason_code` from a closed, versioned enum (`support_ticket`, `fraud_review`, `kyc_reverification`, `legal_hold`) logged with the audit record; adding a value to the enum requires a documented change. Freeform justification text is not acceptable — an unaudit-able reason code is the same gap as no reason code, six months later.
- **`profile:search`**: additionally enforces the per-query result cap; a query that would exceed it unconstrained (e.g., a bare wildcard) is rejected outright, not truncated.

### Pagination, rate limits, and decomposition resistance

- **Cursor-based pagination**, not offset — offset walks turn a capped page size into full-table enumeration. Default page 20, hard ceiling 50.
- **Per-client-credential rate limits**: 60 req/min for `read:own`/`read:any`, 10 req/min for `profile:search`.
- **Cumulative distinct-record-touch counter per caller per rolling window** (e.g., no more than N distinct profiles returned to one credential per 24h without a fresh `reason_code`/justification), tracked independently of per-minute rate. A single-request cap and per-minute throttling only stop single-request or bursty enumeration; a caller stayng under both limits can still walk the namespace in slices and reassemble the population over time (decomposition) — this counter is what catches that pattern regardless of how slowly it's spread. Counting-store mechanism is 03/04's to build; the requirement that volume is measured cumulatively, not just per-minute, is set here.
- Sustained near-limit usage on any of the above alerts, not just hard-blocks at the ceiling.

### Grant lifecycle

- **90-day re-approval** on every scope grant (not `profile:search` alone) is a backstop, not the only mechanism. Grants are revoked immediately, ahead of the 90-day cycle, on: the named approver's offboarding, the caller's stated business function changing, or an idle trigger (no usage for 30 days → revoke pending re-justification) — whichever comes first. This requires 03/04 to consume an offboarding or ownership-change signal; the requirement (lifecycle-triggered revocation, not solely calendar-triggered) is asserted here, the plumbing is theirs.

## Defense in depth beyond the token check

- **TLS 1.2+ only**, terminated in front of the API; no plaintext.
- **Input validation** on search parameters — reject unbounded wildcard queries, cap query complexity, parameterize all DB access (SQL injection is a data-ops/engineering implementation concern, but the API layer's job is to never pass a raw client string into a query string).
- **Audit logging**: every search/retrieve call logs caller identity (`sub`), scope used, record id(s) touched, and timestamp — never the PII field values themselves, per the threat model's log-hygiene requirement. This is what makes a compromised-but-legitimate credential's misuse detectable after the fact, which token-based authn alone does not give you.
- **Standard API hardening**: no PII in URLs (query strings get logged by proxies/CDNs outside our control) — record identifiers in the path are fine, but search terms containing a phone number or name fragment belong in a POST body or a properly-redacted query parameter set, not a GET query string that ends up in a shared access log.

## Why not "just API keys"

Worth stating plainly since it's the simplest alternative a reviewer might expect: a static API key has no expiry, no standard scope representation, and if it leaks (URL, log, repo) it's valid until manually rotated. The chosen design bounds leak exposure to minutes (token TTL) rather than "until someone notices," and expresses authorization natively instead of requiring a bespoke key→permissions side table. That trade costs implementation complexity (an authorization-server component, however minimal) which is why it's stated explicitly here rather than assumed.

## Objections considered (S4 adversarial pair)

Full record: `decisions/search-authz-scoping.md`. Ingrid Solano's five numbered objections against Helena Marsh's initial proposal, all accepted and revised into the sections above: (1) `read:any` was shaped before its actual callers were enumerated — fixed by requiring a caller-role table entry, gated like `search`; (2) `read:any`'s `reason_code` was freeform — fixed with a closed, versioned enum; (3) `caller_referral` membership had no owner or approval process, the softest control gating the narrowest scope — fixed by routing it through the same partner-onboarding workflow as any other grant; (4) per-request rejection and per-minute rate limits don't stop decomposition (slow enumeration under the cap) — fixed with a cumulative distinct-record-touch counter per rolling window; (5) 90-day re-approval is calendar-only, not lifecycle-tied — fixed with immediate revocation on offboarding, function change, or a 30-day idle trigger, with the 90-day cycle as backstop only.

## Sources

- RFC 6749 (OAuth 2.0), §4.4 Client Credentials Grant: https://datatracker.ietf.org/doc/html/rfc6749#section-4.4
- RFC 7519 (JSON Web Token): https://datatracker.ietf.org/doc/html/rfc7519
- OWASP API Security Top 10 2023, API1 (Broken Object Level Authorization) and API3 (Broken Object Property Level Authorization): https://owasp.org/API-Security/editions/2023/en/0x11-t10/
- NIST SP 800-63B (identity/authorization lifecycle, offboarding-triggered revocation) — cited in `decisions/search-authz-scoping.md`
