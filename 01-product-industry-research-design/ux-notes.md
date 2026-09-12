# UX Notes — Two Surfaces on Top of This Backend

Prepared for the `01-product-industry-research-design` track. See `../CLAUDE.md` for the assignment and `./CLAUDE.md` for this track's scope: this is light-touch, backend-supporting narrative, not a design deliverable in its own right — no wireframes, no design canvas. Text, one page, describing screens and the one design rule each surface enforces.

## Surface 1: the admin profile-search console (Persona 2)

**Who uses it:** the support/ops analyst from `personas-use-cases.md`, resolving a ticket or investigating a flagged transaction.

**Screens:**

- **Search screen.** A single form: name (with a partial-match toggle — phone is always exact match, no toggle needed, per the accepted product limitation in `personas-use-cases.md`), phone. No blank-query submit — the form requires at least one field filled before the search button is enabled, matching the "no unconstrained query" product requirement.
- **Results list.** Each row shows masked/partial PII by default (e.g., a partially redacted phone number, a name, no address) and an "expand" affordance per row, not a bulk-select checkbox anywhere on the screen.
- **Expanded record view.** Full PII for one record, reached only through the per-row expand action. This is a different screen state, not a hover-to-reveal or an inline toggle on the results list — the extra navigation step is deliberate and consistent with the backend treating expanded access as a distinct, auditable event.

**The one design rule this surface enforces:** masked-by-default, expand-is-a-distinct-action. Every other screen decision — no bulk-select, expand requiring its own navigation step, no "export" button anywhere — is downstream of that one rule, not a separate design choice each time.

## Surface 2: the onboarding pre-fill flow (Persona 3)

**Who uses it:** a new user signing up for a service that offers "sign in with a partner provider" instead of a blank registration form, per Persona 3 in `personas-use-cases.md`.

**Screens:**

- **Provider choice.** A short list of partner providers (ABC, XYZ, ...), each a single button — the user picks one, consistent with the connector being provider-agnostic on the backend.
- **Partner credential entry.** Username/password fields, framed as "sign in to `<provider>`," not the service's own login form — visually distinct, to signal whose credential this is.
- **Pre-filled review screen.** Name, phone, and address arrive already filled in from the `/identity` response; the user reviews and confirms rather than retyping. Fields are editable, not read-only — the provider is a starting point, not an irrevocable source of truth for this signup.

**The one design rule this surface enforces:** the partner's password is never visible to, or stored by, this service past the token exchange — the review screen never displays a password field, partner or otherwise, and nothing on this screen persists the credential entered on the previous one. That is the UI-level expression of the connector's `/auth` → `/identity` two-step: a short-lived token stands in for the password everywhere past the first screen. That token is held only by this onboarding flow, only for the span between the two calls, and is never persisted past this signup step — the connector that issues it holds it no longer than that either (`../02-ai-security-architecture/connector-security.md` §1).
