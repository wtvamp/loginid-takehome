# API & Connector Design Rationale

Prepared for the `01-product-industry-research-design` track. See `../CLAUDE.md` for the full assignment text and `./CLAUDE.md` for this track's scope. Written so `../03-engineering-delivery/CLAUDE.md` can implement against it directly, without re-deriving the reasoning — that's this document's explicit job per the track brief. Builds on `industry-framing.md` and `personas-use-cases.md` in this directory; read those for the "why," this document is the "so build it this way."

**AI tooling note:** produced by Claude Code (Sonnet 5), track-owner subagent persona Naomi Voss, as a design-reasoning pass — no new external research; synthesizes the assignment text plus this track's prior work.

This document covers three surfaces in the order the assignment presents them: the `user_profile`/`user_credential` data contract (Q1), the search/retrieve REST API (Q2), and the third-party IDP connector (Q3).

## 1. Data contract: `user_profile` and `user_credential`

**Why two separate entities, not one row:** the assignment names them separately, and that separation is doing real work, not just organizing fields. `user_profile` is PII a service holds *about* a person (name, address, phone) — data with governance obligations (retention, right-to-deletion, breach-notification exposure) attached to `../05-data-ops/CLAUDE.md`'s scope. `user_credential` is a proof-of-identity mechanism the service uses *to authenticate* that person — a username, an auth `method`, and (for the password method) a secret. Conflating them into one table would mean every profile read touches the credential store and vice versa, which is exactly the wrong blast radius for a credential breach.

**Why `user_credential.method` matters as a design decision, not just a column:** per `industry-framing.md` §2, this field is the seam where the assignment's deliberately simple password baseline becomes extensible toward what LoginID actually sells (FIDO2/passkey, `method = "webauthn"`) without a schema rewrite. Concretely: `method` should be treated as a discriminator that governs which other fields on the row are meaningful — a `"password"` row has a password hash; a future `"webauthn"` row would have a public key and credential ID instead, not a password field left null by convention. Whether that's modeled as one wide table with nullable method-specific columns or a base table plus method-specific side tables is an implementation choice for `../03-engineering-delivery/CLAUDE.md` and `../05-data-ops/CLAUDE.md` — the product requirement this document is asserting is only that **`method` must be a real discriminator with room to grow, not a fixed enum assumed to always be `"password"`.**

**One relationship, not two independent tables:** a `user_profile` and its `user_credential`(s) need a stable foreign key (one profile can plausibly have more than one credential — a password today, a passkey added later, without replacing the old one until migration is complete). Designing for 1-to-many from the start avoids a breaking migration the day a second auth method is added.

## 2. REST API: search and retrieve

**Endpoint shape, from the personas in `personas-use-cases.md`:**

- `GET /profiles/{id}` — exact retrieval by primary key. Serves any caller who already has the ID (the common, low-risk case).
- `POST /profiles/search` — the search surface Persona 2 (support/ops analyst — the internal user who looks up a customer record by name or phone during a support ticket; see `personas-use-cases.md`) actually uses day to day. **Resolved as `POST`, not `GET` with query params**, for two product-level reasons, not just implementation convenience: (1) the search body needs to carry partial-match flags and an explicit `expand` toggle — a boolean field in the request body, see the Contract Summary below for its exact shape — alongside the query fields, which is awkward to express and easy to get wrong as bare query params; (2) `GET` query strings land in access logs and browser history by default, and a phone number or partial name is PII — `POST` keeps the query payload out of the URL and lines up with where this document already requires every search call to be an audit-log event. This is the endpoint that needs the most deliberate authz design, because "search by phone" is simultaneously the legitimate support workflow and the shape of a scraping attack.

**Why search and retrieve are named as separate concerns in the assignment, and should stay separate in the API design:** retrieval-by-ID is nearly risk-free — if a caller has a valid ID, they likely already have a legitimate reason to look at that record. Search is different: it's a fuzzy-match query over a PII store, which means the API surface itself is the control point. Design implications this document asserts (mechanics belong to `../02-ai-security-architecture/CLAUDE.md`):

- Search results should default to **masked/partial PII** in the response, with full-field detail requiring either a distinct, more tightly scoped call or an explicit "expand" parameter — so that a broad search doesn't silently become a bulk PII export.
- Search should be **rate-limited and paginated** independent of retrieval, because unbounded search is the difference between "an analyst looked up one ticket" and "someone enumerated the table."
- Every search call is a natural audit-log event (who searched, for what, when) even before considering who's authorized to call it at all — this is a data-governance requirement (`../05-data-ops/CLAUDE.md`) that the API design needs to leave room for (e.g., not designing a stateless-only API that makes call attribution an afterthought).

**Why "plus security API authentication" is in the same assignment bullet as search/retrieve, not a separate question:** the assignment is signaling that the two are inseparable — a search/retrieve API over PII without an authentication design isn't a partial answer, it's a wrong one. This document asserts the product-level requirement (search needs scoped, attributable, rate-limited access, not just "logged in"); the actual authn/authz mechanism (API keys vs. OAuth2 client-credentials vs. mTLS, token formats, scope models) is `../02-ai-security-architecture/CLAUDE.md`'s to design.

## 3. Third-party IDP connector: `/auth` and `/identity`

The assignment specifies exact contracts for both endpoints (see root `CLAUDE.md` — `/auth` takes `{"username", "password"}` and returns an `access_token`; `/identity` takes `{"phone", "name"}` and returns PII including a structured address). The design rationale here is about *why* that shape makes sense and *how to build the connector so it isn't hostage to any one provider's quirks* — not about re-specifying the contract, which is already fixed.

**Why the two-call shape (`/auth` then `/identity`) is the right pattern, per Persona 3** (the onboarding flow that pre-fills a new user's profile from a third-party IDP at signup; see `personas-use-cases.md`)**:** it's a minimal, legacy-style token-exchange flow — trade a long-lived credential (username/password) for a short-lived, purpose-scoped token, then use that token for the actual data pull. This is structurally the same shape as OAuth2's **resource-owner-password-credentials (ROPC) grant** — one of several standard ways an OAuth2 client obtains a token, this one taking a raw username/password directly instead of redirecting the user to log in elsewhere — and it is the specific grant type RFC 9700 now states MUST NOT be used, in favor of authorization-code or client-credentials flows (see `research-oauth-history.md` for the OAuth lineage this pattern sits in). Recognizing that lineage matters for the write-up: this connector contract is not a novel design, it's a well-understood, industry-standard-but-dated pattern (see `industry-framing.md` §2, revised after the S2 debate on `PLAN.md` — see `decisions/password-baseline-debate.md` — this is named as the conventional baseline, not as a claim about the assignment author's intent).

**Why the connector must be built provider-agnostic (ABC/XYZ as instances of one interface, not two bespoke integrations):** the assignment names the providers generically on purpose — this is a request to design *a connector interface*, with ABC and XYZ as its first two implementations, not to hardcode either one. Concretely, that means:

- A common internal interface — something like `Authenticate(username, password) → token` and `FetchIdentity(token, phone, name) → PII` — that both ABC and XYZ adapters satisfy, so adding a third provider is "write an adapter," not "touch the calling code."
- Each provider adapter owns its own quirks (token expiry format, field-name differences, rate limits, error codes) behind that common interface — the calling code (the onboarding flow from Persona 3, above) never branches on which provider it's talking to.
- The interface's shape should anticipate that a future provider might not be username/password-based at all — an OAuth2/OIDC provider would satisfy `Authenticate` differently (redirect-based auth-code exchange instead of a direct password POST) but still ultimately produce a token this connector can use the same way for `FetchIdentity`. This is the connector-side equivalent of `user_credential.method`'s extensibility from §1 — flagged here, not built here, per `industry-framing.md`'s point about not burying the actual assignment under speculative scope.

**Field-mapping note (product-level, not implementation):** the assignment's `/identity` response splits address into `street_address`, `locality`, `region`, `postal_code`, `country` — this is recognizably the OpenID Connect `address` claim shape (a widely-used standard structure for exactly this purpose), which is a useful fact for the write-up: it signals the assignment's own contract is already leaning on real industry convention rather than an arbitrary shape, which is worth naming explicitly as evidence of "this connector is meant to look like a real federated-identity integration," reinforcing the design choices above.

## Summary for the engineering track

Three seams to build with deliberately, per this document: `user_credential.method` as a real discriminator (not a fixed enum), a search API designed around masked-by-default results + rate limiting + audit logging (not just an authenticated GET), and a provider-agnostic connector interface with ABC/XYZ as its first two adapters (not two copy-pasted integrations). Each seam is where this "before" baseline visibly points toward the "after" — passkeys, scoped search, and modern federated-identity providers — without this submission having to build any of the "after" itself.

## Contract summary for 03

This section is the single-page version 03 should build against without re-reading the rest of this document. Where this section and the prose above conflict, this section is current.

**Entities (Q1)** — exact assignment field names:

- `user_profile`: `name`, `address`, `phone` (plus a primary key and the FK target for `user_credential`).
- `user_credential`: `username`, `method`, `password` (plus a primary key and a FK to `user_profile`).
- Relationship: **one `user_profile` to many `user_credential`** — do not model as 1:1. `method` is a real discriminator column (string/enum-with-room, not a fixed two-value enum) that determines which other columns on a `user_credential` row are meaningful; only the `"password"` method needs a password hash today. How that's physically modeled (nullable columns vs. method-specific side tables) is 03's and `../05-data-ops/CLAUDE.md`'s call, not fixed here.

**REST API (Q2):**

- `GET /profiles/{id}` — exact retrieval.
- `POST /profiles/search` — search, **not** `GET` with query params (rationale above). Request body shape: a top-level `expand: bool` field, sitting alongside the query fields (`name`, `phone`, partial-match flags), e.g. `{"name": "...", "phone": "...", "expand": false}` — not nested under a sub-object. Default (`expand` absent or `false`) returns masked/partial PII; `expand: true` requests full-field detail and, per the authz mechanism in `../02-ai-security-architecture/CLAUDE.md`, should require a more tightly-scoped credential than the base search call.
- Default response for search is masked/partial PII; `expand: true` is required for full-field detail and should be more tightly scoped than the base search call (same field, same meaning, as above).
- Search is rate-limited and paginated, independently of retrieval-by-ID.
- Every search call must be attributable and audit-logged (caller identity, query, timestamp) — the API shape must not make call attribution an afterthought (e.g., no anonymous/stateless-only search path).
- Authn/authz mechanism (API key vs. OAuth2 client-credentials vs. mTLS, token formats, scopes) is fully specified in `../02-ai-security-architecture/CLAUDE.md` — 03 implements against that document's design, not against an invention of its own.

**Connector interface (Q3):**

- Endpoints, methods, and JSON bodies for `/auth` and `/identity` are fixed by the assignment (see root `CLAUDE.md`) — 03 does not re-specify them.
- Build one provider-agnostic interface — `Authenticate(username, password) → token`, `FetchIdentity(token, phone, name) → PII` — with ABC and XYZ as its first two adapters. The calling code (onboarding flow) never branches on provider identity.
- `/identity` PII response fields (`name`, `phone`, `street_address`, `locality`, `region`, `postal_code`, `country`) match the assignment exactly; the address sub-shape matches the OIDC Core `address` claim (`research-oidc-address-claim.md`) — reuse those member names verbatim in Go structs, do not rename them.
- Leave the interface's `Authenticate` signature able to accommodate a future non-password-based provider (e.g., OAuth2/OIDC redirect flow) without redesigning `FetchIdentity` — flagged as a design constraint on the interface shape, not something to build now.

**Out of scope for 03 — do not re-derive, read the owning track instead:**

- The authn/authz *mechanism* for the search/retrieve API — `../02-ai-security-architecture/CLAUDE.md`.
- How `user_credential.method`'s discriminator is physically modeled (columns vs. side tables) and all schema/migration decisions — `../05-data-ops/CLAUDE.md`.
- The `/auth` and `/identity` field names and JSON shapes — fixed by the assignment text itself, not a design decision any track owns.
- Whether the framing narrative calls the password baseline "deliberate" — it does not, per the S2 structured written debate recorded in `decisions/password-baseline-debate.md` ("S2" = the second story in `PLAN.md`'s task list, not a spec term); 03 need not engage with that question at all, it only affects prose in this track's docs.

