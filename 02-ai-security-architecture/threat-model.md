# Threat Model

Owner: Marcus Ilori (`02-ai-security-architecture`). Scope per this track's `CLAUDE.md`: credential storage, PII exposure, and third-party `access_token` handling. Retention/lifecycle policy is `../05-data-ops/CLAUDE.md`'s call, not mine — I model exposure and access risk, not how long data should live.

## Assumptions, named explicitly

1. **Assets in scope**: `user_credential` (username, method, password — modeled by Priya/`05-data-ops` as a lookup-table `method`, not a native enum, so a given user can have more than one credential row, e.g. password + WebAuthn), `user_profile` (name, address, phone), and third-party `access_token`s obtained via the IDP connector's `/auth`.
2. **`method` is not "always password."** Priya's schema treats `method` as an FK into an `auth_method` table (`id`, `name`, `requires_secret`, `is_active`). That means "credential storage" is not one hashing problem — it's a family of them, one per method, and this document treats it that way rather than assuming a single password column.

### Concrete hashing decision, against the landed schema

`05-data-ops/schema/postgres_cockroachdb.sql` (and its SQLite counterpart) gives `user_credential` three columns for this purpose: `secret` (nullable TEXT, salted hash only — NULL where `auth_method.requires_secret = false`, e.g. passkey/WebAuthn), `hash_algo` (VARCHAR, e.g. `'argon2id'`), and `hash_cost` (INTEGER, the work factor at hash time). This track's concrete decision against those columns:

- **`hash_algo = 'argon2id'`**, per OWASP's and NIST SP 800-63B's current recommendation for password storage. bcrypt is the documented fallback (`hash_algo = 'bcrypt'`) only if the Go implementation's Argon2id library support is unavailable in the chosen stack — a call for `03-engineering-delivery` to confirm, not a reason to default to bcrypt speculatively.
- **Salt handling**: no separate salt column is needed or wanted. Argon2id's standard encoded output (the `$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>` PHC string format) embeds a fresh random salt per row inside the `secret` column's own text — this is what makes the hash self-describing and rehashable without a side lookup. Do not add a `salt` column; it would be redundant with the encoded format and an easy place for an implementation to accidentally desync salt from hash.
- **`hash_cost` at minimum**: memory ≥19 MiB, iterations ≥2, parallelism ≥1 (OWASP's Argon2id floor) — recorded per-row (not just globally) specifically so a future cost bump can be applied to newly-created/rotated rows without forcing every existing row to rehash in lockstep, per the schema's own stated intent of supporting an in-place scheme upgrade.
- **`requires_secret = false` rows** (passkey/WebAuthn, `secret IS NULL`): no hashing question at all — the server holds only public key material for those methods, which is a different asset class (public, not secret) and does not belong in `secret`/`hash_algo`/`hash_cost` at all if/when a passkey method is actually added. Flagged here so `03-engineering-delivery` doesn't try to force a WebAuthn public key through the password-hashing path.
3. **LoginID's own business is passwordless/FIDO2-based authentication.** The assignment nonetheless asks for a `password` field in `user_credential`. I treat that as a deliberate test of whether the submitter still applies correct secret-handling discipline to a legacy-shaped requirement, not as license to under-engineer it. Noted once here; not re-litigated per section.
4. **No real infrastructure exists yet.** This is a design-time threat model for a take-home, not an audit of a running system. Findings are stated as requirements the engineering track (`../03-engineering-delivery/CLAUDE.md`) must implement, not as "confirmed" vulnerabilities.
5. **Trust boundary**: this service is the system of record for `user_profile`/`user_credential` and is a *client* of third-party IDPs (ABC/XYZ) for `/auth` and `/identity`. Those vendors are outside our control; we can only constrain what we do with what they return.

## Method: STRIDE per asset, attacker-centric

I'm using STRIDE (Microsoft, but the categories are now common vocabulary in the field — not claiming Anthropic or LoginID provenance for the framework itself) as the checklist, but organized by asset rather than by data flow diagram element, since that maps directly onto this track's ownership boundaries.

### Asset 1: `user_credential` (password + other auth methods)

| Threat | Scenario | Mitigation (this track's requirement) |
|---|---|---|
| **Information disclosure** — password store compromise | DB dump or SQL injection exposes the `password` column | Never store plaintext or reversibly-encrypted passwords. Argon2id (OWASP Password Storage Cheat Sheet current recommendation, memory-hard, GPU-resistant) with per-row random salt, tuned to ≥19 MiB memory / ≥2 iterations as a floor, re-tuned to the deploying hardware. bcrypt is an acceptable fallback only if Argon2id is unavailable in the chosen Go stack. |
| **Information disclosure** — non-password methods | A WebAuthn public key or TOTP shared secret is not a password, but the table conflates them under one `password` column name | Method-aware storage: WebAuthn credential public keys are not secrets and can be stored in the clear (they're public keys by design) but the associated counter and credential ID must not be attacker-writable; TOTP shared secrets ARE secrets and must be encrypted at rest (not hashed — the server needs to recover the value to verify), using envelope encryption with a KMS-held key, mechanics owned by `../04-infra-devops/CLAUDE.md`. |
| **Elevation of privilege** — credential stuffing / brute force | Attacker replays leaked username/password pairs against `/auth`-adjacent endpoints | Rate limiting and progressive backoff per username and per source IP; account lockout or step-up challenge after a threshold; never reveal via response timing or error text whether the *username* or the *password* was wrong (user enumeration). |
| **Tampering** | Attacker modifies `method` or the credential row to downgrade a user from a strong method (WebAuthn) to a weak one (password) they control | Method changes/additions require re-authentication with an *existing* registered method (step-up), never a bare authenticated-session write. |
| **Repudiation** | No record of who created/rotated a credential | Audit log of credential creation/rotation events (actor, timestamp, method type — never the secret itself). |

### Asset 2: `user_profile` (name, address, phone — PII)

| Threat | Scenario | Mitigation |
|---|---|---|
| **Information disclosure via the search API** | Question 2 asks for "search and retrieve." An authenticated-but-unscoped search endpoint is a PII enumeration engine — an attacker with any valid token can page through the entire user table by name/phone fragments | Search must be authorization-scoped (a caller sees only records it's entitled to, not the whole table) and rate-limited/paginated; see `api-auth-design.md` for the concrete authz model. This is the single highest-severity finding in this document, precisely because it's easy to build the search feature "to spec" and miss that spec implies a data-exfiltration primitive. |
| **Information disclosure in transit** | PII served over an unencrypted or downgradable channel | TLS 1.2+ mandatory, HSTS, no plaintext fallback. Mechanics of certificate issuance are `04-infra-devops`'s call; the requirement is this track's. |
| **Information disclosure via logs** | PII (address, phone) written to application logs, error traces, or APM breadcrumbs | Structured logging with an explicit PII-field denylist; log the *fact* of a profile access (who, when, which record id) never the field values themselves. |
| **Tampering** | Unauthorized profile field modification | Same authz model as read access; write operations additionally require the resource owner or an admin-scoped credential, not just "any authenticated caller." |

### Asset 3: Third-party `access_token` (from the IDP connector, question 3)

| Threat | Scenario | Mitigation |
|---|---|---|
| **Information disclosure** — token at rest | `access_token` from ABC/XYZ cached or persisted in plaintext | Treat it as a secret with the same bar as our own credentials: encrypted at rest if persisted at all, short TTL, never written to application logs (a full request/response log of `/auth` is an anti-pattern — vendor password *and* resulting token both land in the same log line). |
| **Information disclosure** — token in transit / replay | Token intercepted and replayed against the vendor or against us | Vendor calls over TLS only; tokens treated as bearer secrets — anyone holding one can act as us to the vendor, so token scope should be minimized to `/identity` only if the vendor's API supports scoped tokens. |
| **Spoofing** | A malicious or compromised proxy sits between us and ABC/XYZ | Certificate validation not disabled "for convenience" in any environment, including local dev — a design note the engineering track should carry into implementation. |
| **Denial of service via credential exhaustion** | Our own credentials to call ABC/XYZ (needed to obtain the `access_token` in the first place) are hardcoded or checked into source | Secrets management is `04-infra-devops`'s mechanism, but the requirement — never in source, never in a committed config file, injected at runtime — is set here. |

## What this track deliberately does not resolve

- How long a cached `/identity` result may be retained — that's data governance (`../05-data-ops/CLAUDE.md`).
- Which specific secrets manager or KMS product is used — that's `../04-infra-devops/CLAUDE.md`; I only require that one exists and that secrets never live in source or plaintext config.
- Whether the product should offer passwordless auth as the primary path (it should, per LoginID's own market position, and `../01-product-industry-research-design/CLAUDE.md` makes that case) — that's product framing, not a threat.

## Sources

- OWASP Password Storage Cheat Sheet (Argon2id as current default recommendation): https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
- OWASP API Security Top 10 (particularly API1:2023 Broken Object Level Authorization and API3:2023 Broken Object Property Level Authorization — both directly relevant to the search/retrieve endpoint finding above): https://owasp.org/API-Security/editions/2023/en/0x11-t10/
- STRIDE threat modeling categories, Microsoft: https://learn.microsoft.com/en-us/azure/security/develop/threat-modeling-tool-threats
