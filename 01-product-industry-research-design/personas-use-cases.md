# Personas & Use Cases

Prepared for the `01-product-industry-research-design` track. See `../CLAUDE.md` for the full assignment text and `./CLAUDE.md` for this track's scope. Ties directly back to the three assignment questions and to `industry-framing.md` in this directory.

**AI tooling note:** produced by Claude Code (Sonnet 5), track-owner subagent persona Naomi Voss, as a design-reasoning pass over the assignment text and this track's prior research — no new external research for this document.

"Who is this for" comes before "how do we build it" — so each persona below is answered before the corresponding assignment question is treated as a design problem.

## Persona 1: The internal service developer (drives Q1 — the DAO)

**Who:** an engineer on a different internal team — say, an onboarding flow, a support tool, or a billing system — who needs to read or write a user's profile or credential record, but should never touch SQL or know which database backend is in play this quarter.

**Why they need this:** the assignment explicitly requires multi-database support (PostgreSQL/CockroachDB, SQLite, etc. — see root `CLAUDE.md`). That requirement only makes sense if there's a real reason the backing store might change: a startup prototyping on SQLite that needs to graduate to CockroachDB for multi-region durability without a data-layer rewrite, or a team standardizing on Postgres while a legacy service still runs SQLite. The DAO's job is to make that migration invisible to every consumer like this persona — they call `GetUserProfile(id)`, not `SELECT * FROM user_profile WHERE id = $1`.

**What this means for Q1's design:** the DAO's interface is the product, not the SQL underneath it. It should be judged on whether a consumer can be written against the interface alone, never learning which database is live. `user_credential.method` existing as a field (not assumed to always be `"password"`) is this persona's forward-compatibility guarantee — when passkeys get added, this developer's code that reads `user_credential.method` to branch on auth type doesn't need to change shape, only add a case.

## Persona 2: The support/ops analyst using the search API (drives Q2 — the REST service)

**Who:** a customer-support agent or fraud/ops analyst who needs to look up a user profile by partial information — a phone number a customer just read aloud, a name with uncertain spelling, an account ID from a ticket — to resolve a support case or investigate a flagged transaction. Not a developer; a consumer of a UI/API surface someone else built on top of this service.

**Why they need "search," specifically, not just "retrieve":** retrieval-by-ID is the easy 80%; a support workflow rarely starts with a clean ID. It starts with whatever the customer said on the phone. That's why Q2 says "search **and** retrieve" — the API needs a query surface (name, phone, partial match) in addition to an exact-key lookup, and that query surface is exactly the thing that needs the tightest authz scoping, because "search by phone number" over a PII store is also the exact capability a bad actor wants.

**What this means for Q2's design:** the API's authentication isn't just "is this caller allowed to hit the service" — it's "what can this caller search for and see," because a support analyst's legitimate access pattern (look up one ticket's customer) looks identical, mechanically, to a scraping attack (enumerate the user table) unless the API is designed to distinguish them by identity, scope, and rate — the mechanics of which belong to `../02-ai-security-architecture/CLAUDE.md`, but the *reason* the distinction matters is this persona: a real support tool has real, narrow, audited search needs, not "return everything."

**Secondary persona for Q2 — the admin/back-office console.** Per this track's "does not own" boundary, this is light-touch: if a UI ever sits on top of the search API, it's an internal console for exactly Persona 2's workflow — a search box, a result list with masked PII by default (full PII visible only on deliberate expand + audit log entry), and no bulk-export button. That's a UX rationale note, not a wireframe deliverable for this backend-focused assignment.

## Persona 3: The onboarding product owner needing third-party PII (drives Q3 — the IDP connector)

**Who:** a product owner building a signup/onboarding flow for a service that wants to pre-fill a new user's name, phone, and address rather than making them type it — because the user already has an account with a partner identity provider (the generic "ABC"/"XYZ" in the assignment), and re-typing the same PII into every new service is friction the industry has been trying to design away since long before OAuth existed (see `research-oauth-history.md` for that lineage).

**Why a service pulls PII from a third party rather than owning it:** two converging reasons, both real. First, **reduced liability and reduced collection surface** — a service that never independently collects and stores a user's address avoids being a second copy of that PII to secure and eventually breach; it becomes a consumer of an assertion instead of a second source of truth (this is the same reasoning behind "federated identity" broadly — don't collect what you can verify from someone who already has it). Second, **conversion/UX** — fewer form fields at signup measurably reduces drop-off, which is why "sign in with X" patterns dominate consumer onboarding.

**What this means for Q3's design:** the `/auth` → `/identity` two-step (get a token, then use the token to fetch PII) is the connector pattern doing exactly what it should — a **short-lived, scoped credential** stands in for the user's actual third-party password, so the calling service never sees or stores that password, only a token it can use for the narrow purpose of one identity lookup. The design rationale for treating `ABC`/`XYZ` as pluggable providers behind a common connector interface (rather than two bespoke integrations) is in `api-connector-design-rationale.md` — but the product reason it must be pluggable is this persona: onboarding flows routinely need to support more than one partner IDP, and adding "sign in with a third provider" should be an integration, not a redesign.

## Cross-cutting observation

All three personas share one property worth stating plainly: **none of them are the end user whose data this is.** The person whose name/phone/address sits in `user_profile`, and whose credential sits in `user_credential`, never directly calls any of these three interfaces — a developer, a support analyst, and a product owner do, on that person's behalf (with consent, in the legitimate cases). That's precisely why authz-by-caller-identity (not just authn) matters throughout, and why the third-party connector's job is explicitly framed as retrieving *someone else's* PII by proxy — the trust boundary is the whole point of the design, not an afterthought bolted onto a CRUD service.
