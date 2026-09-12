# IDP Connector Security Design — Question 3

Owner: Marcus Ilori (`02-ai-security-architecture`). Covers the third-party identity-provider connector (generic vendors "ABC"/"XYZ") exposing `/auth` and `/identity`, per the assignment. Implementation target: `../03-engineering-delivery/CLAUDE.md`.

## Contract recap (from the assignment, verbatim in `../CLAUDE.md`)

- `POST /auth` — body `{"username": "<string>", "password": "<string>"}` → returns an `access_token`.
- `POST /identity` — body `{"phone": "<string>", "name": "<string>"}` → returns PII (name, phone, address fields).

Read literally, this connector is a thin proxy in front of ABC/XYZ: it takes vendor credentials, gets a vendor token, and uses that token to pull PII from the vendor. Every security question here is "what does this service do with a vendor secret and vendor PII while it's in our custody," since we don't control the vendor's own security posture.

## Threat surface specific to this connector

This extends the third-party-token row in `threat-model.md`; this document is the concrete design against those threats.

### 1. What happens to the vendor `access_token` once retrieved

- **Never logged.** The `/auth` request body contains a vendor password; the response contains a bearer token. A naive "log the request and response for debugging" habit puts both in the same log line — this is called out explicitly because it's the single most common way a connector like this leaks a credential in practice, not a hypothetical.
- **Held in memory for the shortest span that satisfies the immediate `/identity` call**, not persisted to our own database as a matter of course. If caching is required for latency/rate-limit reasons, cache with a TTL no longer than the vendor's own token expiry, encrypted at rest (same envelope-encryption requirement as the TOTP-secret case in the threat model — mechanics via KMS, owned by `../04-infra-devops/CLAUDE.md`), and keyed so one vendor-account's token is never returned to a caller resolving a different vendor account.
- **Scoped narrowly if the vendor supports it** — request only the scope needed for `/identity`, not a broad token, so a leaked token's blast radius is bounded by the vendor's own authorization model as well as ours.
- **Not returned to our own API callers.** The vendor `access_token` is an internal implementation detail of this connector; question 2's API callers never see it. If a caller needs to know "identity lookup succeeded," return our own success/failure signal, not the vendor's token or vendor's raw response.

### 2. How `/identity` calls are authenticated to the vendor

- Standard bearer-token usage: the vendor `access_token` from `/auth` presented as an `Authorization: Bearer` header to the vendor's identity endpoint (or whatever the vendor's own contract specifies — this connector adapts to the vendor, not the reverse).
- **Token refresh/re-auth on expiry** handled by the connector transparently — a caller of our `/identity` endpoint should not need to know or manage vendor token lifecycle.
- **Vendor calls over TLS only**, certificate validation never disabled, including in local/dev environments (repeating the threat model's point because it's the most common place a "temporary" dev bypass ships to production unnoticed).
- **Our own credentials to call the vendor's `/auth`** (this connector's client id/secret with ABC/XYZ, if the vendor requires connector-level registration in addition to the end-user's own username/password) are runtime-injected secrets, never in source or config committed to the repo — requirement set here, mechanism owned by `../04-infra-devops/CLAUDE.md`.

### 3. What this service should and shouldn't log or persist

| Data | Log it? | Persist it? |
|---|---|---|
| End-user's vendor username | No (identifier, but low sensitivity relative to password — still avoid in plaintext logs; use it only for correlation via a hash if audit trails need it) | Only if a legitimate business reason exists; not by default |
| End-user's vendor password (`/auth` request body) | **Never, under any circumstance, including error paths and stack traces** | **Never** |
| Vendor `access_token` | **Never** | Only as short-TTL encrypted cache per above; never in plaintext, never in application logs |
| PII returned by `/identity` (name, phone, address fields) | Never the field values; log only the *fact* of a lookup (caller identity, timestamp, success/failure, which record was resolved by opaque id) | Retention/caching policy for this PII is `../05-data-ops/CLAUDE.md`'s call — this track's requirement is that whatever is persisted follows the same encryption-at-rest and access-control bar as `user_profile` data, no weaker because it arrived via a connector rather than direct input |
| Vendor error responses | Log status/error-code only; do not log raw vendor response bodies by default, since a vendor error payload can itself echo back submitted PII or partial credentials | N/A |

### 4. Failure handling as a security control, not just reliability

- **Vendor auth failures must not leak which part of the credential was wrong** (username vs. password) in whatever this connector surfaces upward — same user-enumeration concern as the threat model's credential-stuffing row, now facing outward at whoever calls our `/auth` passthrough.
- **Retry/backoff on vendor calls** should be rate-limited on our side independent of the vendor's own limits, so a compromised or careless caller can't use our connector to hammer the vendor (which is both an availability risk to us and a way to get our connector's vendor credentials rate-limited or banned).
- **Circuit-breaking**: if the vendor is failing or behaving anomalously (e.g., returning tokens for identities that don't match the request), fail closed — do not fall back to a cached or stale identity result silently, since that risks serving stale PII as if freshly verified.

## What this design does not resolve

- Data retention duration for cached `/identity` results — `../05-data-ops/CLAUDE.md`.
- Which secrets manager holds this connector's own vendor credentials — `../04-infra-devops/CLAUDE.md`.
- Vendor selection/evaluation (which real-world providers "ABC"/"XYZ" might represent) — that's product framing in `../01-product-industry-research-design/CLAUDE.md`, not a security question.

## Sources

- OWASP API Security Top 10 2023, API7 (Server Side Request Forgery) and API8 (Security Misconfiguration) — both relevant to a service that makes outbound calls to a third party on a user's behalf: https://owasp.org/API-Security/editions/2023/en/0x11-t10/
- OWASP Logging Cheat Sheet (what not to log — credentials, tokens, PII): https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html
