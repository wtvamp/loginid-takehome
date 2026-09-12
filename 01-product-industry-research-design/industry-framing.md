# Industry & Competitive Framing

Prepared for the `01-product-industry-research-design` track. See `../CLAUDE.md` for the full assignment text and `./CLAUDE.md` for this track's scope and boundaries. Builds directly on `research-loginid-company.md` and `research-oauth-history.md` in this directory — read those first for sourcing; this document is the synthesized argument, not new research.

**AI tooling note:** produced by Claude Code (Sonnet 5) running as a standing track-owner subagent for this track (persona: Naomi Voss), reading this track's own CLAUDE.md as its brief and the two prior research passes as its evidence base. No new web research was done for this document — it is a synthesis pass over material already gathered.

## Who is this for

Two audiences read this document, and the framing below is written to serve both without contradiction:

1. **LoginID's technical evaluators.** They already know their own market. What they're grading here is whether a candidate can place a deliberately simple assignment (password-based credentials, a bare username/password IDP connector) accurately on the map of where the industry actually is — and say so plainly rather than either quietly building an insecure relic or over-engineering a FIDO2 flow nobody asked for.
2. **Whoever would actually consume this system if it were built for real** — an internal team building a profile-search admin tool, or a product team wiring up an onboarding flow that calls out to a third-party IDP connector. This document's persona/use-case section (see `personas-use-cases.md`) exists because "who is this for" has a real answer beyond the interview panel, and that real answer should shape the design.

## 1. Where this assignment sits in the identity/IAM landscape

The three questions asked — a DAO for `user_profile`/`user_credential`, a searchable REST API over that data, and a connector to third-party identity providers using `/auth` (username/password → token) and `/identity` (token → PII) — describe a **classic CIAM (Customer Identity and Access Management) backend**: the boring, load-bearing plumbing that sits underneath every "log in" button. This space is large, mature, and crowded:

- **Full-platform CIAM**: Okta/Auth0, Microsoft Entra ID, Ping Identity — passwordless is one feature among many (SSO, MFA, lifecycle management, B2B federation).
- **Passkey/FIDO2 specialists**: LoginID itself, HYPR, Beyond Identity, Transmit Security, Corbado, Hanko, Descope — narrower, deeper on one authentication method.
- **IDP-aggregation/identity-verification vendors**: services whose entire business is "collect PII from a person once and let many relying parties query it" — the same shape as the assignment's `/identity` endpoint, at a bigger scale.

The assignment sits at the intersection of the first and third categories, but built at the "baseline reference implementation" layer rather than the polished-product layer — which tracks with the assignment's own instructions that code doesn't need to be complete, and that reasoning matters more than a finished product.

## 2. The deliberate gap: what LoginID sells vs. what LoginID is asking for

This is the framing that most needs to be said out loud, because it's easy to miss and, once seen, it reframes the whole submission.

LoginID's actual product is FIDO2/WebAuthn passkey authentication: asymmetric device-bound credentials unlocked by biometrics, with **no shared secret ever transmitted or stored server-side** (see `research-loginid-company.md` §1). The assignment, by contrast, specifies:

- `user_credential (username, method, password)` — a shared-secret password field, the exact thing passkeys exist to eliminate.
- A third-party `/auth` endpoint authenticating with `{"username", "password"}` — a legacy password-grant pattern, not an OAuth2/OIDC delegated-authorization flow.

**This is the conventional, industry-standard credential baseline** used across CIAM backends generally, not a signal we're reading as unique to LoginID — a resolved S2 debate concluded that "LoginID chose this gap deliberately" is an unfalsifiable claim about the assignment author's intent, with no artifact in the assignment text to distinguish it from an ordinary take-home template (see `decisions/password-baseline-debate.md`). We don't need that inference to justify what matters here, which is building the baseline correctly regardless of who asked for it:

- **Name the gap explicitly** rather than silently building an insecure password store as if it were best practice, or silently substituting a FIDO2 flow the assignment didn't ask for (which would answer a different question than the one asked).
- **Design the seams so the gap is closeable later without a rewrite.** The `user_credential.method` field is the natural extension point — a value like `"password"` today, `"webauthn"` or `"passkey"` tomorrow, without changing the table shape. The third-party connector's pluggable-provider design (see `api-connector-design-rationale.md`) is the equivalent seam on the IDP side — a modern OIDC/OAuth2 provider should slot in next to the ABC/XYZ password-grant providers without a redesign.
- **Still handle what's actually being asked with the rigor the brand implies** — hashed/salted passwords at minimum if a password field must exist at all, least-privilege API auth on search/retrieve, secrets hygiene for stored third-party access tokens. (Full mechanics of this are `../02-ai-security-architecture/CLAUDE.md`'s scope; this document only asserts the product-narrative reason it matters — LoginID's whole brand *is* auth security, so an unhashed password field in the reference implementation, however illustrative, would read as tone-deaf.)

In short: **the assignment's design is the industry-standard "before" baseline; LoginID's product is one credible "after."** The strongest submission narrative is "here is a solid, correctly-built baseline, with the seams marked for where it could evolve toward passkeys" — stated as forward-looking architecture, not as a claim about what the assignment's author had in mind.

## 3. The adjacent frontier: agentic identity, and why it's relevant but out of scope

Two things are converging on LoginID's roadmap that are each worth naming, but neither should be built into this submission's actual code:

- **LoginID's own 2025–2026 repositioning** toward "agentic commerce" — authenticating AI agents transacting on a human's behalf, with scoped/signed/revocable mandates (`research-loginid-company.md` §1, §3). This is company-stated strategy, not independently confirmed, and it's a genuine pivot from "passkey login for any site" to "identity/payment-authorization layer for AI agents."
- **AAuth / the broader machine-identity protocol space** (`research-oauth-history.md`) — a proposal (Dick Hardt, editor of OAuth 2.0/RFC 6749, not a co-author of OAuth 1.0/RFC 5849 — see that document for why the precise credential matters) addressing a different layer of the stack: authorizing *HTTP clients/agents* to services, versus FIDO2's job of authenticating *humans* to services. The two are complementary, not competing — a mature identity platform plausibly needs both, with passkey-based human authentication anchoring the root of trust that agent-identity protocols delegate from.

**Relevance to this assignment:** the same critique AAuth levels at static, pre-registered client credentials (API keys that leak, no portable identity, one-time upfront consent rather than per-request authorization) applies loosely to why a third-party-IDP connector built as a static username/password integration is a deliberately dated pattern — and why the connector's provider abstraction should be designed with an eye toward OAuth2/OIDC-style delegated flows as the natural next step, the same way `user_credential.method` is the seam for FIDO2 on the human-authentication side.

This belongs in the write-up as forward-looking context, not as scope creep — nobody asked for an agent-identity protocol implementation, and building one would bury the actual assignment under speculative complexity. One paragraph in the submission's narrative, pointing at the seam, is the right amount of ambition.

## 4. Competitive takeaway for the submission's narrative

Framed as a one-line thesis for whoever reads the final packet: *this submission is a correctly-scoped, industry-standard credential baseline, built with explicit extension points toward FIDO2/passkey credentials, OIDC-style IDP federation, and (further out) agent-identity delegation — not because we claim to know that's what LoginID intended by the gap, but because those are the same extension points any CIAM backend eventually needs, and naming them shows the same market fluency without resting on an unfalsifiable read of the assignment author's mind.*
