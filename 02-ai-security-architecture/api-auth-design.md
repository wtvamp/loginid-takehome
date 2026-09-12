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

- **Scopes**, carried in the JWT `scope` claim, e.g. `profile:read:own`, `profile:read:any`, `profile:search`. A given client-credential is provisioned with the narrowest scope its actual use case requires — most integrations should get `profile:read:own` (retrieve a specific known record by id, e.g. resolving a record the caller already has a legitimate reference to) rather than `profile:search` (query by name/phone fragments across the table), which should be reserved for a small number of explicitly-provisioned back-office callers.
- **Object-level authorization is enforced per request, not just per token.** Holding `profile:read:any` scope is necessary but not sufficient — every retrieve/search call is logged (see below) and, where the caller model supports it, additionally checked against a resource-level policy (e.g., a partner integration scoped to only its own referred users) rather than trusting the coarse scope alone. This directly targets OWASP API1:2023 (Broken Object Level Authorization), cited in the threat model.
- **Search results are paginated with a hard page-size ceiling** and rate-limited per client, independent of the scope check — a legitimately-scoped caller doing an anomalously large number of sequential searches is still a signal worth throttling and alerting on, not just permitting because the token was valid.

## Defense in depth beyond the token check

- **TLS 1.2+ only**, terminated in front of the API; no plaintext.
- **Input validation** on search parameters — reject unbounded wildcard queries, cap query complexity, parameterize all DB access (SQL injection is a data-ops/engineering implementation concern, but the API layer's job is to never pass a raw client string into a query string).
- **Audit logging**: every search/retrieve call logs caller identity (`sub`), scope used, record id(s) touched, and timestamp — never the PII field values themselves, per the threat model's log-hygiene requirement. This is what makes a compromised-but-legitimate credential's misuse detectable after the fact, which token-based authn alone does not give you.
- **Standard API hardening**: no PII in URLs (query strings get logged by proxies/CDNs outside our control) — record identifiers in the path are fine, but search terms containing a phone number or name fragment belong in a POST body or a properly-redacted query parameter set, not a GET query string that ends up in a shared access log.

## Why not "just API keys"

Worth stating plainly since it's the simplest alternative a reviewer might expect: a static API key has no expiry, no standard scope representation, and if it leaks (URL, log, repo) it's valid until manually rotated. The chosen design bounds leak exposure to minutes (token TTL) rather than "until someone notices," and expresses authorization natively instead of requiring a bespoke key→permissions side table. That trade costs implementation complexity (an authorization-server component, however minimal) which is why it's stated explicitly here rather than assumed.

## Sources

- RFC 6749 (OAuth 2.0), §4.4 Client Credentials Grant: https://datatracker.ietf.org/doc/html/rfc6749#section-4.4
- RFC 7519 (JSON Web Token): https://datatracker.ietf.org/doc/html/rfc7519
- OWASP API Security Top 10 2023, API1 (Broken Object Level Authorization) and API3 (Broken Object Property Level Authorization): https://owasp.org/API-Security/editions/2023/en/0x11-t10/
